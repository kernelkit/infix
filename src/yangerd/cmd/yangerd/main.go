package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/backoff"
	"github.com/kernelkit/infix/src/yangerd/internal/collector"
	"github.com/kernelkit/infix/src/yangerd/internal/config"
	"github.com/kernelkit/infix/src/yangerd/internal/containermonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/dbusmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/ethmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/frrvty"
	"github.com/kernelkit/infix/src/yangerd/internal/fswatcher"
	"github.com/kernelkit/infix/src/yangerd/internal/ipbatch"
	"github.com/kernelkit/infix/src/yangerd/internal/ipc"
	"github.com/kernelkit/infix/src/yangerd/internal/iwmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/lldpmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/ptpmonitor"
	"github.com/kernelkit/infix/src/yangerd/internal/monitor"
	"github.com/kernelkit/infix/src/yangerd/internal/sysreaders"
	"github.com/kernelkit/infix/src/yangerd/internal/tftpmonitor"
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

func (osFileChecker) ListDir(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
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

	slogLog := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel(cfg.LogLevel)}))

	var wg sync.WaitGroup
	// spawn runs a monitor until ctx ends; an exit before that is a bug
	// worth a log line, the monitor itself owns its restarts.
	spawn := func(name string, run func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := run(ctx); err != nil && ctx.Err() == nil {
				slogLog.Error(name+" exited", "err", err)
			}
		}()
	}
	cmd := collector.ExecRunner{}
	fs := collector.OSFileReader{}
	hardware := collector.NewHardwareCollector(cmd, fs, cfg.PollHardware, cfg.EnableWifi, cfg.EnableGPS)
	t.RegisterProvider("ietf-hardware:hardware", hardware.Live)
	collectors := []collector.Collector{
		collector.NewSystemCollector(cmd, fs, cfg.PollSystem),
		collector.NewRoutingCollector(cmd, cfg.PollRouting),
		collector.NewNTPCollector(cmd, cfg.PollNTP),
		hardware,
	}
	pokes := collector.RunAll(ctx, &wg, t, collectors)

	inst := collector.DBusInstaller{}
	t.RegisterProvider("ietf-system:system-state", func() json.RawMessage {
		live := collector.LiveSystemState(fs)
		installerOverlay := collector.MergeInstaller(t.GetCached("ietf-system:system-state"), inst)
		if installerOverlay == nil {
			return live
		}
		return tree.ShallowMerge(live, installerOverlay)
	})

	if data := collector.BootPlatform(fs); data != nil {
		t.Merge("ietf-system:system-state", data)
	}
	if data := collector.BootSoftware(ctx, cmd); data != nil {
		t.Merge("ietf-system:system-state", data)
	}

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

	brBatch, err := ipbatch.NewBridge(ctx, slogLog)
	if err != nil {
		log.Fatalf("start bridge batch: %v", err)
	}
	defer brBatch.Close()

	nlmon := monitor.New(linkBatch, addrBatch, neighBatch, brBatch, t, osFileChecker{}, slogLog)

	// inotify says nothing when a /proc/sys/net/*/conf/<if> directory
	// comes or goes, so follow the interface set from netlink instead.
	linkSetCh := make(chan struct{}, 1)
	nlmon.SetLinkSetChange(func() {
		select {
		case linkSetCh <- struct{}{}:
		default:
		}
	})

	ethMon := ethmonitor.New(slogLog, cmd)
	ethMon.SetOnUpdate(nlmon.SetEthernetData)
	nlmon.SetEthRefresh(ethMon.RefreshInterface)
	spawn("ethmonitor", ethMon.Run)

	spawn("wireguard", poll(nlmon.WaitReady(), 10*time.Second, func() {
		nlmon.SetWireguardAll(wgquery.Query(nlmon.Links()))
	}))
	spawn("stp", poll(nlmon.WaitReady(), cfg.PollSTP, nlmon.RefreshSTP))
	spawn("nlmonitor", func(ctx context.Context) error {
		return backoff.Retry(ctx, slogLog, "nlmonitor", nlmon.Run)
	})

	if cfg.EnableWifi {
		iwmon := iwmonitor.New(slogLog)
		iwmon.SetOnUpdate(nlmon.SetWifiData)
		iwmon.SetOnPhyChange(func() { pokes.Poke(hardware.Name()) })
		spawn("iwmonitor", iwmon.Run)
	}

	if cfg.EnableLLDP {
		lldpmon := lldpmonitor.New(t, slogLog)
		spawn("lldpmonitor", lldpmon.Run)
	}

	if cfg.EnableContainers {
		ctrmon := containermonitor.New(t, cmd, fs, slogLog)
		spawn("containermonitor", ctrmon.Run)
	}

	tftpmon := tftpmonitor.New(t, slogLog)
	spawn("tftpmonitor", tftpmon.Run)

	ptpmon := ptpmonitor.New(t, slogLog)
	spawn("ptpmonitor", ptpmon.Run)

	zapi := zapiwatcher.New(t, frrvty.New(""), slogLog)
	spawn("zapiwatcher", zapi.Run)

	if cfg.EnableDHCP || cfg.EnableFirewall {
		dbusMon := dbusmonitor.New(t, slogLog)
		spawn("dbusmonitor", dbusMon.Run)
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
	forwarding := fswatcher.WatchHandler{
		TreeKey:  routingTreeKey,
		ReadFunc: fwdAgg.HandleForwardingChange,
		Debounce: 100 * time.Millisecond,
		UseMerge: true,
	}
	syncForwarding := func() {
		for _, pattern := range forwardingPaths {
			if _, _, err := fsw.SyncGlob(pattern, forwarding); err != nil {
				slogLog.Warn("fswatcher glob failed", "pattern", pattern, "err", err)
			}
		}
	}
	syncForwarding()

	spawn("forwarding-sync", func(ctx context.Context) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-linkSetCh:
				syncForwarding()
			}
		}
	})

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
	// Container operational data is handled by containermonitor (a
	// `podman events` stream), not the fswatcher.
	fsw.InitialRead()
	spawn("fswatcher", fsw.Run)

	go func() {
		<-nlmon.WaitReady()
		ready.Store(true)
		// finit's notify:pid marks the service ready when this appears
		if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
			slogLog.Warn("write pidfile", "path", pidFile, "err", err)
		}
	}()
	defer os.Remove(pidFile)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		for sig := range sigCh {
			if sig == syscall.SIGHUP {
				log.Printf("SIGHUP: triggering immediate re-poll")
				pokes.PokeAll()
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

// poll runs fn once ready is closed and then every interval until the
// context ends.
func poll(ready <-chan struct{}, every time.Duration, fn func()) func(context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ready:
		}
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			fn()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	}
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

const (
	routingTreeKey = "ietf-routing:routing"
	pidFile        = "/run/yangerd.pid"
)

func makeBootOrderReader(t *tree.Tree, cmd collector.CommandRunner) func(string) (json.RawMessage, error) {
	return func(_ string) (json.RawMessage, error) {
		bootOrder := collector.ReadBootOrder(context.TODO(), cmd)

		raw := t.GetCached("ietf-system:system-state")
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
