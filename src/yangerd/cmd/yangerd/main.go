package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/bridgebatch"
	"github.com/kernelkit/infix/src/yangerd/internal/collector"
	"github.com/kernelkit/infix/src/yangerd/internal/config"
	"github.com/kernelkit/infix/src/yangerd/internal/dbusmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/ethmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/frrvty"
	"github.com/kernelkit/infix/src/yangerd/internal/fswatcher"
	"github.com/kernelkit/infix/src/yangerd/internal/ipbatch"
	"github.com/kernelkit/infix/src/yangerd/internal/ipc"
	"github.com/kernelkit/infix/src/yangerd/internal/iwmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/lldpmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/monitor"
	"github.com/kernelkit/infix/src/yangerd/internal/sysreaders"
	"github.com/kernelkit/infix/src/yangerd/internal/tree"
	"github.com/kernelkit/infix/src/yangerd/internal/wgquery"
	"github.com/kernelkit/infix/src/yangerd/internal/zapiwatcher"
)

// osFileChecker implements iface.FileChecker using the real filesystem.
type osFileChecker struct{}

func (osFileChecker) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (osFileChecker) ReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func main() {
	cfg := config.Load()
	log.SetFlags(0)

	t := tree.New()
	ready := &atomic.Bool{}

	srv := ipc.NewServer(t, ready)
	if err := srv.Listen(cfg.Socket); err != nil {
		log.Fatalf("listen %s: %v", cfg.Socket, err)
	}
	defer os.Remove(cfg.Socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	cmd := collector.ExecRunner{}
	fs := collector.OSFileReader{}
	collectors := []collector.Collector{
		collector.NewSystemCollector(cmd, fs, cfg.PollSystem),
		collector.NewRoutingCollector(cmd, cfg.PollRouting),
		collector.NewNTPCollector(cmd, cfg.PollNTP),
		collector.NewHardwareCollector(cmd, fs, cfg.PollHardware, cfg.EnableWifi, cfg.EnableGPS),
	}
	pokeCh := make(chan struct{}, len(collectors))
	collector.RunAll(ctx, &wg, t, collectors, pokeCh)

	inst := collector.DBusInstaller{}
	t.RegisterProvider("ietf-system:system-state", func() json.RawMessage {
		live := collector.LiveSystemState(fs)
		installerOverlay := collector.MergeInstaller(t.GetCached("ietf-system:system-state"), inst)
		if installerOverlay == nil {
			return live
		}
		var base map[string]json.RawMessage
		if json.Unmarshal(live, &base) != nil {
			return live
		}
		var overlay map[string]json.RawMessage
		if json.Unmarshal(installerOverlay, &overlay) != nil {
			return live
		}
		for k, v := range overlay {
			base[k] = v
		}
		merged, err := json.Marshal(base)
		if err != nil {
			return live
		}
		return merged
	})

	if data := collector.BootPlatform(fs); data != nil {
		t.Merge("ietf-system:system-state", data)
	}
	if data := collector.BootSoftware(ctx, cmd); data != nil {
		t.Merge("ietf-system:system-state", data)
	}

	slogLog := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel(cfg.LogLevel)}))

	linkBatch, err := ipbatch.New(ctx, slogLog, ipbatch.WithStats(), ipbatch.WithDetails())
	if err != nil {
		log.Fatalf("start link batch: %v", err)
	}
	defer linkBatch.Close()

	addrBatch, err := ipbatch.New(ctx, slogLog, ipbatch.WithDetails())
	if err != nil {
		log.Fatalf("start addr batch: %v", err)
	}
	defer addrBatch.Close()

	neighBatch, err := ipbatch.New(ctx, slogLog)
	if err != nil {
		log.Fatalf("start neigh batch: %v", err)
	}
	defer neighBatch.Close()

	brBatch, err := bridgebatch.New(ctx, slogLog)
	if err != nil {
		log.Fatalf("start bridge batch: %v", err)
	}
	defer brBatch.Close()

	nlmon := monitor.New(linkBatch, addrBatch, neighBatch, brBatch, t, osFileChecker{}, slogLog)

	ethMon, err := ethmonitor.New(slogLog, cmd)
	if err != nil {
		slogLog.Warn("ethmonitor unavailable, continuing without it", "err", err)
	} else {
		ethMon.SetOnUpdate(nlmon.SetEthernetData)
		nlmon.SetEthRefresh(ethMon.RefreshInterface)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ethMon.Run(ctx); err != nil && ctx.Err() == nil {
				slogLog.Error("ethmonitor exited", "err", err)
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		<-nlmon.WaitReady()
		for {
			links := nlmon.Links()
			for ifname, data := range wgquery.Query(links) {
				nlmon.SetWireguardData(ifname, data)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(cfg.PollSTP)
		defer ticker.Stop()
		<-nlmon.WaitReady()
		for {
			nlmon.RefreshSTP()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if err := nlmon.Run(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				slogLog.Error("nlmonitor exited, restarting in 1s", "err", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			} else {
				return
			}
		}
	}()

	if cfg.EnableWifi {
		iwmon := iwmonitor.New(slogLog)
		iwmon.SetOnUpdate(nlmon.SetWifiData)
		iwmon.SetOnPhyChange(func() {
			select {
			case pokeCh <- struct{}{}:
			default:
			}
		})
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := iwmon.Run(ctx); err != nil && ctx.Err() == nil {
				slogLog.Error("iwmonitor exited", "err", err)
			}
		}()
	}

	if cfg.EnableLLDP {
		lldpmon := lldpmonitor.New(t, slogLog)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lldpmon.Run(ctx); err != nil && ctx.Err() == nil {
				slogLog.Error("lldpmonitor exited", "err", err)
			}
		}()
	}

	zapi := zapiwatcher.New(t, frrvty.New(""), slogLog)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := zapi.Run(ctx); err != nil && ctx.Err() == nil {
			slogLog.Error("zapiwatcher exited", "err", err)
		}
	}()

	if cfg.EnableDHCP || cfg.EnableFirewall {
		dbusMon := dbusmonitor.New(t, slogLog)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := dbusMon.Run(ctx); err != nil && ctx.Err() == nil {
				slogLog.Error("dbusmonitor exited", "err", err)
			}
		}()
	}

	fsw, err := fswatcher.New(t, slogLog)
	if err != nil {
		log.Fatalf("start fswatcher: %v", err)
	}

	fwdAgg := sysreaders.NewForwardingAggregator()
	forwardingPaths := []string{
		"/proc/sys/net/ipv4/conf/*/forwarding",
		"/proc/sys/net/ipv6/conf/*/forwarding",
	}
	for _, pattern := range forwardingPaths {
		matches, globErr := filepath.Glob(pattern)
		if globErr != nil {
			slogLog.Warn("fswatcher glob failed", "pattern", pattern, "err", globErr)
			continue
		}
		for _, path := range matches {
			if err := fsw.Watch(path, fswatcher.WatchHandler{
				TreeKey:  routingTreeKey,
				ReadFunc: fwdAgg.HandleForwardingChange,
				Debounce: 100 * time.Millisecond,
				UseMerge: true,
			}); err != nil {
				slogLog.Warn("fswatcher watch failed", "path", path, "err", err)
			}
		}
	}
	if err := fsw.Watch("/etc/hostname", fswatcher.WatchHandler{
		TreeKey:  "ietf-system:system",
		ReadFunc: sysreaders.ReadHostname,
		Debounce: 200 * time.Millisecond,
		UseMerge: true,
	}); err != nil {
		slogLog.Warn("fswatcher watch failed", "path", "/etc/hostname", "err", err)
	}
	if err := fsw.WatchSymlink("/etc/localtime", fswatcher.WatchHandler{
		TreeKey:  "ietf-system:system",
		ReadFunc: sysreaders.ReadTimezone,
		Debounce: 200 * time.Millisecond,
		UseMerge: true,
	}); err != nil {
		slogLog.Warn("fswatcher watch failed", "path", "/etc/localtime", "err", err)
	}
	usersHandler := fswatcher.WatchHandler{
		TreeKey:  "ietf-system:system",
		ReadFunc: sysreaders.ReadUsers,
		Debounce: 200 * time.Millisecond,
		UseMerge: true,
	}
	if err := fsw.Watch("/etc/shadow", usersHandler); err != nil {
		slogLog.Warn("fswatcher watch failed", "path", "/etc/shadow", "err", err)
	}
	if err := fsw.WatchDir(sysreaders.SSHDKeysDir, usersHandler); err != nil {
		slogLog.Warn("fswatcher watch failed", "path", sysreaders.SSHDKeysDir, "err", err)
	}
	bootOrderHandler := fswatcher.WatchHandler{
		TreeKey:  "ietf-system:system-state",
		ReadFunc: makeBootOrderReader(t, cmd),
		Debounce: 200 * time.Millisecond,
		UseMerge: true,
	}
	// Watch the parent directory, not the file: fw_setenv (U-Boot) and
	// grub-editenv may rewrite the env via a temp file + rename, which
	// gives it a new inode that a direct file watch never sees.  Watching
	// the directory catches the Create/Rename (and still catches in-place
	// writes), so a boot-order change after a RAUC install is reflected
	// without waiting for a reboot.
	for _, path := range []string{"/mnt/aux/grub/grubenv", "/mnt/aux/uboot.env"} {
		if err := fsw.WatchSymlink(path, bootOrderHandler); err != nil {
			slogLog.Debug("fswatcher boot-order watch skipped", "path", path, "err", err)
		}
	}
	dnsHandler := fswatcher.WatchHandler{
		TreeKey:  "ietf-system:system-state",
		ReadFunc: sysreaders.ReadDNSResolver,
		Debounce: 200 * time.Millisecond,
		UseMerge: true,
	}
	for _, path := range []string{"/etc/resolv.conf.head", "/var/lib/misc/resolv.conf"} {
		if err := fsw.WatchSymlink(path, dnsHandler); err != nil {
			slogLog.Warn("fswatcher dns watch failed", "path", path, "err", err)
		}
	}
	if cfg.EnableContainers {
		containerHandler := fswatcher.WatchHandler{
			TreeKey: "infix-containers:containers",
			ReadFunc: func(_ string) (json.RawMessage, error) {
				return collector.CollectContainers(cmd, fs), nil
			},
			Debounce: 500 * time.Millisecond,
		}
		os.MkdirAll("/run/libpod/events", 0755)
		if err := fsw.WatchDir("/run/libpod/events", containerHandler); err != nil {
			slogLog.Warn("fswatcher container watch failed", "err", err)
		}
	}
	fsw.InitialRead()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := fsw.Run(ctx); err != nil && ctx.Err() == nil {
			slogLog.Error("fswatcher exited", "err", err)
		}
	}()

	go func() {
		<-nlmon.WaitReady()
		ready.Store(true)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		for sig := range sigCh {
			if sig == syscall.SIGHUP {
				log.Printf("SIGHUP: triggering immediate re-poll")
				for range len(collectors) {
					pokeCh <- struct{}{}
				}
				continue
			}
			cancel()
			return
		}
	}()

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("serve: %v", err)
	}

	wg.Wait()
}

func slogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

const routingTreeKey = "ietf-routing:routing"

func makeBootOrderReader(t *tree.Tree, cmd collector.CommandRunner) func(string) (json.RawMessage, error) {
	return func(_ string) (json.RawMessage, error) {
		bootOrder := collector.ReadBootOrder(context.TODO(), cmd)

		raw := t.Get("ietf-system:system-state")
		var state map[string]interface{}
		if raw != nil {
			json.Unmarshal(raw, &state)
		}
		if state == nil {
			state = make(map[string]interface{})
		}

		sw, _ := state["infix-system:software"].(map[string]interface{})
		if sw == nil {
			sw = make(map[string]interface{})
		}

		if bootOrder != nil {
			sw["boot-order"] = bootOrder
		} else {
			delete(sw, "boot-order")
		}

		return json.Marshal(map[string]interface{}{"infix-system:software": sw})
	}
}
