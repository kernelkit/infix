#!/bin/sh

sleep 30

stress-ng --cpu 8 --io 4 --vm 2 --vm-bytes 128M --fork 4 -t 0
