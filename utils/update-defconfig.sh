#!/bin/sh
# Update a defconfig derived from a standard defconfig, like 'make oldconfig'

set -e

SCRIPT_DIR="$(readlink -f "$(dirname -- "$0")")"
MERGE_CONFIG="${SCRIPT_DIR}/../buildroot/support/kconfig/merge_config.sh"
DIFFCONFIG="${SCRIPT_DIR}/../buildroot/utils/diffconfig"
TAB="$(printf '\t')"

usage() {
    cat <<EOF
Usage: $(basename "$0") [-y] BASE DEFCONFIG
Carry over changes made to a base defconfig since the last run to a
defconfig derived from it, prompting for each.  The base as of the last
run is kept in DEFCONFIG.base, which is created on the first run.

Arguments:
    BASE                     Base defconfig, e.g. infix/configs/aarch64_defconfig
    DEFCONFIG                Derived defconfig to update

Options:
    -y, --yes                Do not prompt, follow the base configuration
EOF
    exit 1
}

quiet() {
    "$@" > "$TMPDIR/log" 2>&1 || { cat "$TMPDIR/log" >&2; exit 1; }
}

# expand FILE DEFCONFIG...: full .config of DEFCONFIG... in FILE
expand() {
    out="$1"
    shift
    quiet "$MERGE_CONFIG" -O "$TMPDIR" "$@"
    cp "$TMPDIR/.config" "$out"
}

# value CONFIG SYMBOL: the value of SYMBOL in CONFIG, n if unset
value() {
    { sed -n "s/^$2=//p; s/^# $2 is not set\$/n/p" "$1"; echo n; } | head -n 1
}

follow() {
    [ -z "$yes" ] || return 0

    desc=$(grep -rh -A4 --include='Config.in*' "^config $1\$" "$SCRIPT_DIR/.." 2>/dev/null \
               | sed -n 's/^[[:space:]]*[a-z]*[[:space:]]*"\(.*\)".*/\1/p' | head -n 1)
    echo
    echo "$1${desc:+ - $desc}"
    printf '          ours: %s  (%s)\n' "$2" "$output"
    printf '  old upstream: %s  (%s)\n' "$3" "$old"
    printf '  new upstream: %s  (%s)\n' "$4" "$base"
    if [ "$2" = "$3" ]; then
        question="Use new upstream?"
    else
        question="Use new upstream (overwrite our modification)?"
    fi
    while :; do
        printf '%s [Y/n] ' "$question"
        read -r ans || { ans=y; echo; }
        case "$ans" in
            ""|y|Y) return 0 ;;
            n|N)    return 1 ;;
        esac
    done
}

yes=""
case "$1" in
    -y|--yes) yes=1; shift ;;
esac
[ $# -eq 2 ] || usage
base="$1"
output="$2"
old="$output.base"

if [ ! -f "$old" ]; then
    cp "$base" "$old"
    echo "$old: created, tracking $base from now on"
    exit 0
fi

if cmp -s "$old" "$base"; then
    echo "$output: up to date"
    exit 0
fi

echo "$output: checking what is new in $base ..."

TMPDIR=`mktemp -d`
trap 'rm -rf "$TMPDIR"' EXIT

expand "$TMPDIR/old.config" "$old"
expand "$TMPDIR/new.config" "$base"
expand "$TMPDIR/ours.config" "$output"

"$DIFFCONFIG" "$TMPDIR/old.config" "$TMPDIR/new.config" | sed 's/^[-+ ]//; s/ .*//' > "$TMPDIR/changed"

cp "$output" "$TMPDIR/ours"
while read -r name <&3; do
    # Only symbols set in a base defconfig, the rest follow from those
    grep -q "^\(# \)\?$name[= ]" "$old" "$base" || continue

    mine=$(value "$TMPDIR/ours.config" "$name")
    from=$(value "$TMPDIR/old.config" "$name")
    to=$(value "$TMPDIR/new.config" "$name")
    [ "$mine" != "$to" ] || continue
    follow "$name" "$mine" "$from" "$to" || continue

    sed -i "/^$name=/d; /^# $name is not set$/d" "$TMPDIR/ours"
    if [ "$to" = "n" ]; then
        echo "# $name is not set" >> "$TMPDIR/ours"
    else
        echo "$name=$to" >> "$TMPDIR/ours"
    fi
done 3< "$TMPDIR/changed"

expand "$TMPDIR/ours.config" "$TMPDIR/ours"
quiet make O="$TMPDIR" savedefconfig
mv "$TMPDIR/defconfig" "$output"
cp "$base" "$old"
echo "$output: updated"
