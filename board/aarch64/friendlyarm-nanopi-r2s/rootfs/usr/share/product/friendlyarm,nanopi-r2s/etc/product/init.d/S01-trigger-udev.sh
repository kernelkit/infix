#!/bin/sh
udevadm control --reload-rules
udevadm trigger --subsystem-match=net --action=add
