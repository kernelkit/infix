#!/bin/bash
#
# Extract the latest release entry from ChangeLog.md and unwrap lines
# for GitHub's web view (lets the browser handle line wrapping).
#
# Usage: cat doc/ChangeLog.md | extract-changelog.sh > release.md
#
# Output goes to stdout.
#

set -e -o pipefail

# Extract section between first two --- lines, skip trailing blank lines
head -n -1 < <(awk '/^-----*$/{if (x == 1) exit; x=1;next}x') |

# Unwrap lines for GitHub's web view (browser does wrapping)
# Preserves: blank lines, list items, headings, blockquotes, indented blocks
# Joins: soft-wrapped paragraph/list continuations
awk '
{
    if (/^$/) {
        if (buf) print buf
        buf = ""
        print
        blank = 1
        next
    }
    if (/^  / && buf) {
        # Indented continuation - strip indent and join
        sub(/^  /, " ")
        buf = buf $0
    } else if (/^[-*>#\[|]/ || /^[0-9]+\./ || blank || !buf || brk) {
        # New block: special char, after blank, first line, or after md break
        if (buf) print buf
        buf = $0
    } else {
        # Paragraph continuation - join with space
        buf = buf " " $0
    }
    blank = 0
    brk = /  $/
}
END { if (buf) print buf }
'

# Append tip for GNS3
cat <<EOF

> [!TIP]
> **Try Infix in GNS3!** Download the appliance from the [GNS3 Marketplace](https://gns3.com/marketplace/appliances/infix) to test Infix in a virtual network environment without hardware.
EOF
