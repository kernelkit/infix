#!/bin/sh

set -e

python3 -m venv ~/.infix-test-venv
cp -r  ~/yang ~/.infix-test-venv/yangdir
. ~/.infix-test-venv/bin/activate

python3 -m pip install --upgrade pip
python3 -m pip install -r "$1"
