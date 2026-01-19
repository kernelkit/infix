#!/bin/sh
# Workaround for: https://github.com/kernelkit/infix/issues/1357
udevadm control --reload-rules
udevadm trigger --subsystem-match=net --action=add
