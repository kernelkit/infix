#!/bin/sh
# Populate the shared Buildroot download cache (dl/) with the source
# artifacts needed by all defconfigs, then optionally publish it to a
# download mirror using rsync.  The dl/ directory layout is exactly what
# BR2_PRIMARY_SITE expects (dl/<package>/<tarball>), the mirror root is
# a plain copy of dl/, see configs/snippets/mirror.conf
#
# Intended to run nightly from cron on the file server, on every branch
# that should remain buildable from the mirror:
#
#   0 3 * * * cd $HOME/src/infix && git pull --ff-only -q && \
#             ./utils/mirror-sync.sh /srv/pub
#

set -u

usage()
{
	cat <<-EOF >&2
	Usage: $0 [-o DIR] [DEST]

	Run 'make <defconfig> source' for every defconfig in configs/,
	downloading all required source artifacts to the shared dl/
	directory.  All tarball hashes are verified by Buildroot.

	If DEST is given, dl/ is then published there with rsync.  Git
	caches (dl/*/git) are skipped and nothing is ever deleted from
	DEST -- old release branches still reference old tarballs.

	Options:
	  -o DIR    Directory for scratch build trees.  Speeds up
	            repeated runs (default: temporary, removed on exit)
	EOF
	exit "$1"
}

outdir=
while getopts "ho:" opt; do
	case $opt in
	h) usage 0;;
	o) outdir=$OPTARG;;
	*) usage 1;;
	esac
done
shift $((OPTIND - 1))
dest=${1:-}

cd "$(dirname "$0")/.." || exit 1

exec 9> "${TMPDIR:-/tmp}/infix-mirror-sync.lock"
if ! flock -n 9; then
	echo "$0: another instance is already running, skipping." >&2
	exit 0
fi

if [ -z "$outdir" ]; then
	outdir=$(mktemp -d)
	trap 'rm -rf "$outdir"' EXIT INT TERM
fi

rc=0
for cfg in configs/*_defconfig; do
	cfg=$(basename "$cfg")
	O="$outdir/${cfg%_defconfig}"

	echo "=== $cfg"
	if ! make O="$O" "$cfg" >/dev/null; then
		echo "$0: failed configuring $cfg, skipping." >&2
		rc=1
		continue
	fi
	if ! make O="$O" source; then
		echo "$0: failed downloading sources for $cfg." >&2
		rc=1
	fi

	# Drop this defconfig's build tree; the downloads live in the
	# shared dl/ dir, and keeping every tree would exhaust the disk.
	rm -rf "$O"
done

if [ -n "$dest" ]; then
	rsync -a --exclude='/*/git' --exclude='*.lock' dl/ "$dest" || rc=1
fi

exit $rc
