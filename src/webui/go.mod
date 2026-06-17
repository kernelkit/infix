module infix/webui

go 1.22.0

toolchain go1.22.2

require github.com/openconfig/goyang v1.6.3

require github.com/google/go-cmp v0.7.0 // indirect

// kernelkit/goyang fork carrying our YANG 1.1 fixes: reference on Value,
// multiple uses-augments, and must in rpc input/output.
replace github.com/openconfig/goyang => github.com/kernelkit/goyang v1.6.4-0.20260617163501-afcacf84230c
