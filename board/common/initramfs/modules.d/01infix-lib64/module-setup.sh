#!/bin/bash

check() {
    return 0
}

depends() {
    return 0
}

install() {
    # Not sure if this should be handled by buildroot's merged-usr
    # module. Fix it here for now.
    ln -s "lib" "${initdir?}/lib64"
}
