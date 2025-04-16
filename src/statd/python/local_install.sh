#!/bin/bash
# installs yanger-tools locally on your computer
poetry build
pip install --break-system-packages --user --force-reinstall dist/infix_yang_tools-1.0-py2.py3-none-any.whl 
rm -rf dist
