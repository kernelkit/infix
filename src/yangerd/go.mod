module github.com/kernelkit/infix/src/yangerd

go 1.26.0

require (
	github.com/facebook/time v0.0.0-20261028182048-956c8cce511a
	github.com/godbus/dbus/v5 v5.2.4
	github.com/mdlayher/genetlink v1.4.0
	github.com/mdlayher/netlink v1.11.2
	github.com/vishvananda/netlink v1.3.1
	github.com/vishvananda/netns v0.0.5
	golang.org/x/sys v0.49.0
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/mdlayher/socket v0.6.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
)

require github.com/kernelkit/infix/src/features v0.0.0

replace github.com/kernelkit/infix/src/features => ../features
