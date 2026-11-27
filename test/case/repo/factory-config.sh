#!/usr/bin/env bash
# Verify the factory-config of every product validates against the models
# The NETCONF server models come from a built tree, skipped without one

SCRIPT_PATH="$(dirname "$(readlink -f "$0")")"
TOPDIR="$(readlink -f "$SCRIPT_PATH/../../..")"
CONFD="$TOPDIR/src/confd"
OUT="$(readlink -f "${O:-$TOPDIR/output}")"
BUILT="$OUT/target/usr/share/yang/modules"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

skip()
{
    printf "1..1\nok 1 # skip %s\n" "$1"
    exit 0
}

[ -d "$BUILT/libnetconf2" ] && [ -d "$BUILT/netopeer2" ] || skip "no built tree with YANG models in $BUILT"
YANGLINT=$(command -v yanglint || echo "$OUT/host/bin/yanglint")
[ -x "$YANGLINT" ] || skip "yanglint not found"

# Module files by name and name@rev, the confd ones first
dirs=("$CONFD/yang/confd")
for dir in "$BUILT"/*/; do
    [ "$(basename "$dir")" = confd ] || dirs+=("${dir%/}")
done
declare -A path
for dir in "${dirs[@]}"; do
    for yang in "$dir"/*.yang; do
	mod=${yang##*/}
	mod=${mod%.yang}
	for key in "$mod" "${mod%@*}"; do
	    [ -n "${path[$key]}" ] || path[$key]=$yang
	done
    done
done

# All modules from the srload .inc files, with the union of their features
declare -A file features
for inc in "$CONFD"/yang/*.inc; do
    # shellcheck disable=SC1090
    mapfile -t entries < <(MODULES=(); . "$inc"; printf '%s\n' "${MODULES[@]}")
    for entry in "${entries[@]}"; do
	read -r -a words <<< "$entry"
	mod="${words[0]%.yang}"
	name="${mod%@*}"
	[ -n "${path[$mod]}" ] || continue
	file[$name]="${path[$mod]}"
	for word in "${words[@]:1}"; do
	    [ "$word" = -e ] || features[$name]+="${features[$name]:+,}$word"
	done
    done
done

# A yang-library file lets yanglint compile the context once, not per module
for name in "${!file[@]}"; do
    flat=$(tr -s '\n\t' '  ' < "${file[$name]}")
    rev=$(grep -o 'revision *"\?[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}' <<< "$flat" | head -1)
    ns=$(grep -o 'namespace *"\?[a-z][a-z0-9+.-]*:[^"; ]*' <<< "$flat" | head -1)
    printf '%s %s %s %s\n' "$name" "${rev: -10}" "${ns##*[ \"]}" "${features[$name]}"
done | jq -R -s '
  [split("\n")[] | select(length > 0) | split(" ") |
   {name: .[0], revision: .[1], namespace: .[2]} +
   (if (.[3] // "") != "" then {feature: (.[3] | split(",") | unique)} else {} end)]
  | {"ietf-yang-library:yang-library": {
       "module-set": [{name: "complete", module: .}],
       "schema": [{name: "complete", "module-set": ["complete"]}],
       "datastore": [{name: "ietf-datastores:running", schema: "complete"}],
       "content-id": "1"},
     "ietf-yang-library:modules-state": {"module-set-id": "1"}}' > "$tmp/yang-library.json"

paths=()
for dir in "${dirs[@]}"; do
    paths+=(-p "$dir")
done
args=(-D "${paths[@]}" -Y "$tmp/yang-library.json" -t config)

merge()
{
    jq -s 'reduce .[] as $item ({}; . * $item)' "$@"
}

# The defaults collated like gen-config, product generators go on top
version=$(sed -n 's/^AC_INIT(\[confd\], *\[\([^]]*\)\].*/\1/p' "$CONFD/configure.ac")
sh "$CONFD/bin/gen-hostname" > "$tmp/20-hostname.json"
sed "s/@VERSION@/$version/" "$CONFD/bin/gen-version.in" | sh > "$tmp/20-version.json"
merge "$CONFD"/share/factory.d/*.json "$tmp"/20-*.json > "$tmp/default.json"

num=0
fail=0
validate()
{
    local cfg="$1" name="$2"

    num=$((num + 1))
    # yanglint also searches its working directory for modules
    if (cd "$tmp" && "$YANGLINT" "${args[@]}" "$cfg") > "$tmp/log" 2>&1; then
	echo "ok $num - $name"
    else
	echo "not ok $num - $name"
	sed 's/^/# /' "$tmp/log"
	fail=1
    fi
}

mapfile -t products < <(find "$TOPDIR/board" -type d -path '*/etc/product/bin' | LC_ALL=C sort)
echo "1..$((1 + ${#products[@]}))"

validate "$tmp/default.json" "factory-config defaults"
for dir in "${products[@]}"; do
    product=${dir%/etc/product/bin}
    gen=()
    for g in gen-ifs-custom gen-cfg-custom; do
	[ -x "$dir/$g" ] || continue
	sh "$dir/$g" > "$tmp/$g.json"
	gen+=("$tmp/$g.json")
    done
    merge "$tmp/default.json" "${gen[@]}" > "$tmp/product.json"
    validate "$tmp/product.json" "factory-config ${product##*/}"
done

exit $fail
