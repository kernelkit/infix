#!/bin/sh

set -e

usage()
{
    local me="$(basename $0)"

    cat <<EOF
usage: $me [<OPTIONS>] [<REV>]

Download the artifact of the specified revision, or HEAD if not
specified, that was built by GitHub's CI action; prepare a new output
tree (by applying the corresponding defconfig); and extract the
artifact.

  <REV>
    Locate an artifact built from this revision. By default, look for
    an artifact built from the current HEAD.

  Options:

    -a <ARCH>
      Locate an artifact built for <ARCH>. By default, look for an
      artifact built for the x86_64 architecture.

    -b <BR>
      Create a new Git branch from <revision> named <BR>, and switch
      to it. By default, stay on the current branch if <REV> is
      "HEAD", otherwise switch to one named "gh-dl-<REV>".

    -h
      Show this help message and exit.

    -o <OUTPUT-DIR> Prepare the artifact in <OUTPUT-DIR>. By default,
      the artifact is extracted in an output directory called
      "x-gh-dl-<REV>-<ARCH>".


  Examples:

    Test the latest image for PR #666:
      gh pr checkout 666
      $0
      cd x-artifact-a1b2c3d4-x86_64
      make run

EOF
}

ixdir=$(readlink -f $(dirname "$(readlink -f "$0")")/..)
. $ixdir/board/common/lib.sh

workdir=$(mktemp --tmpdir -d infix-gh-dl-XXXXXXXX)
cleanup()
{
    rm -rf $workdir
}
trap cleanup EXIT

topdir=$(git rev-parse --show-toplevel)

apibase="repos/{owner}/{repo}"
arch=x86_64
rev=$(git rev-parse HEAD)
br=
outdir=

# getopt
while getopts "a:b:ho:" opt; do
    case ${opt} in
	a)
	    arch="${OPTARG}"
	    ;;
	b)
	    br="${OPTARG}"
	    ;;
	h)
	    usage && exit
	    ;;
	o)
	    outdir="${OPTARG}"
	    ;;
    esac
done
shift $((OPTIND - 1))

echo $br

case $# in
    0)
	;;
    1)
	rev=$(git rev-parse $1 2>/dev/null || true)
	[ "$1" = "HEAD" ] || br=__DEFAULT
	;;
    *)
	usage && exit 1
	;;
esac


ixmsg "Locating $arch build of $rev"

sha=$(gh api "$apibase/commits/$rev" -q .sha || die "ERROR: Unknown revision \"$rev\"")
echo "SHA: $sha"

runs=$(gh api "$apibase/actions/runs?head_sha=$sha" \
	 -q '.workflow_runs[] | .id' | tr '\n' ' ' \
	   || die "ERROR: Found no workflow runs associated with $rev")

echo "Runs: $runs"

for run in $runs; do
    gh api "$apibase/actions/runs/$run/artifacts" \
       -q ".artifacts[] | select(.name == \"artifact-$arch\")" >$workdir/artifact.json \
	|| continue

    artifact="$(jq .id $workdir/artifact.json)"
    [ "$artifact" ] && break
done

[ "$artifact" ] || die "ERROR: Found no $arch artifact associated with any of the runs $runs"
echo "Artifact: $artifact"

url="$(jq -r .archive_download_url $workdir/artifact.json)"
slug=$(echo $sha | head -c 8)

outdir=${outdir:-$topdir/x-gh-dl-$slug-$arch}
imgdir=$outdir/images


if [ "$br" ]; then
    if [ "$br" = "__DEFAULT" ]; then
	br=gh-dl-$slug
    fi

    ixmsg "Checking out $slug to branch $br"

    if ! git cat-file -e $sha 2>/dev/null; then
	echo Artifact is built from $slug, which is not available locally. Updating all remotes...
	git remote update

	if ! git cat-file -e $sha 2>/dev/null; then
	    die "ERROR: Unable to locate $slug"
	fi
    fi

    git checkout --recurse-submodules -B $br $sha
fi

ixmsg "Setting up output directory $outdir"
make -C $topdir O=$outdir ${arch}_defconfig


ixmsg "Downloading artifact"
zip=$workdir/zip
gh api $url >$zip


ixmsg "Extracting artifact"
mkdir -p $imgdir
unzip -p $zip infix-$arch.tar.gz | gunzip | tar -C $imgdir -x --strip-components=1


ixmsg "Done"
echo "From $outdir, you can now execute \`make run\`, " \
     "\`make test\` etc., using artifact $artifact" | fmt
