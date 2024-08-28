#!/bin/sh
# shellcheck disable=SC1090

set -e

mkdir -p        ~/.infix/venv
python3 -m venv ~/.infix/venv
cp -r ~/yang    ~/.infix/venv/yangdir
.               ~/.infix/venv/bin/activate

python3 -m pip install --upgrade pip
python3 -m pip install -r "$1"
