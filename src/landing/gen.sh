#!/bin/sh
# Generate index.html from Infix README.md

BASE=../../../../..
TITLE="Welcome to Infix :-)"

cp $BASE/doc/logo.png .
cat $BASE/README.md \
    | tail +2 \
    | sed 's/doc\/logo.png/logo.png/' \
    | pandoc -f markdown+implicit_figures+link_attributes -o index.html \
	     --metadata pagetitle="$TITLE" --template=hpstr-template.html
