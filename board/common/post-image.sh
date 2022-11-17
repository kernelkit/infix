#!/bin/sh

common=$(dirname $(readlink -f "$0"))

$common/mkfit.sh
