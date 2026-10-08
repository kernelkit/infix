package tftpmonitor

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// watchMounts calls notify every time the mount table changes, until ctx
// is cancelled.  A mount produces no inotify event, but the kernel flags
// /proc/self/mountinfo with POLLPRI when the table changes, see proc(5).
// The file has to be read to the end after each change to re-arm it.
func watchMounts(ctx context.Context, path string, notify func()) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var wake [2]int
	if err := unix.Pipe2(wake[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return fmt.Errorf("pipe: %w", err)
	}
	defer unix.Close(wake[0])
	defer unix.Close(wake[1])

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			unix.Write(wake[1], []byte{0})
		case <-stop:
		}
	}()

	drain := func() {
		f.Seek(0, io.SeekStart)
		io.Copy(io.Discard, f)
	}
	drain()

	fds := []unix.PollFd{
		{Fd: int32(f.Fd()), Events: unix.POLLPRI},
		{Fd: int32(wake[0]), Events: unix.POLLIN},
	}
	for {
		fds[0].Revents, fds[1].Revents = 0, 0
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == unix.EINTR {
				continue
			}
			return fmt.Errorf("poll %s: %w", path, err)
		}
		if fds[1].Revents != 0 {
			return ctx.Err()
		}
		if fds[0].Revents&(unix.POLLPRI|unix.POLLERR) != 0 {
			drain()
			notify()
		}
	}
}
