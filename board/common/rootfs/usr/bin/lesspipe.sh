#!/bin/sh

case "$1" in
    *.gz)
	zcat "$1"
	;;
    *)
	cat "$1"
	;;
esac
