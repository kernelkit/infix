module github.com/kernelkit/webui

go 1.22.0

toolchain go1.22.2

require (
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/openconfig/goyang v1.6.3 // indirect
	github.com/pborman/getopt v1.1.0 // indirect
)

// Local fork of goyang with YANG 1.1 fixes:
//   - Uses.Augment: *Augment → []*Augment (multiple augments per uses)
//   - Value: add Reference field (when { reference "..."; })
//   - Input/Output: add Must field (must statements in rpc input/output)
replace github.com/openconfig/goyang => ./internal/goyang
