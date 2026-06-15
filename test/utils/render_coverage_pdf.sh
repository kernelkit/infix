#!/bin/sh
# Render the XPath coverage markdown report to PDF using pandoc + weasyprint.
#
# Prepends an HTML cover page and replaces the emoji status markers with
# CSS-styled spans, then hands the result to pandoc.  Runs inside the
# infix-test container, where pandoc, weasyprint and the fonts live.
#
# Usage:
#   render_coverage_pdf.sh <report.md> <report.css> <logo.png> <version> <output.pdf>
set -e

md="$1"
css="$2"
logo="$3"
ver="$4"
pdf="$5"

gen=$(sed -n 's/^Generated: //p' "$md" | head -1 | cut -d' ' -f1)

{
    printf '<div class="cover">\n'
    printf '<img class="cover-logo" src="%s">\n' "$logo"
    printf '<h1 class="cover-title">XPath Coverage Report</h1>\n'
    printf '<p class="cover-version">%s</p>\n' "$ver"
    printf '<p class="cover-date">%s</p>\n' "$gen"
    printf '</div>\n\n'
    sed -e 's,🟢,<span class="dot ok"></span>,g'   \
        -e 's,🟠,<span class="dot warn"></span>,g' \
        -e 's,🔴,<span class="dot err"></span>,g'  \
        -e 's,✅,<span class="mark ok">✓</span>,g'  \
        -e 's,❌,<span class="mark err">✗</span>,g' \
        -e '/^# XPath Coverage Report$/d'           \
        -e '/^Generated: /d'                        \
        "$md"
} > "$md.pdf.md"

pandoc "$md.pdf.md" \
    -f gfm \
    --pdf-engine=weasyprint \
    -c "$css" \
    -o "$pdf"

rm -f "$md.pdf.md"
