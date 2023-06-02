#!/bin/sh

set -e

python3 -m venv ~/.infix-test-venv
. ~/.infix-test-venv/bin/activate

python3 -m pip install --upgrade pip
python3 -m pip install -r "$1"
