#!/bin/sh
# RESTCONF CLI wrapper for curl

# Show usage and exit
usage()
{
	cat <<-EOF >&2
	Usage: $0 [-h HOST] [-d DATASTORE] [-u USER:PASS] METHOD PATH [CURL_ARGS...]

	Options:
	  -h HOST    Target host (default: infix.local)
	  -d DS      Datastore: running, operational, startup (default: running)
	  -u CREDS   Credentials as user:pass (default: admin:admin)

	Methods: GET, POST, PUT, PATCH, DELETE
	EOF
	exit "$1"
}

# Default values
HOST=${HOST:-infix.local}
DATASTORE=running
AUTH=admin:admin

# Parse options
while getopts "h:d:u:" opt; do
	case $opt in
		h) HOST="$OPTARG" ;;
		d) DATASTORE="$OPTARG" ;;
		u) AUTH="$OPTARG" ;;
		*) usage 1 ;;
	esac
done
shift $((OPTIND - 1))

# Validate required arguments
if [ $# -lt 2 ]; then
	echo "Error: METHOD and PATH are required" >&2
	usage 1
fi

METHOD=$1
PATH=$2
shift 2

# Ensure PATH starts with /
case "$PATH" in
	/*) ;;
	*) PATH="/$PATH" ;;
esac

# Build URL based on datastore
case "$DATASTORE" in
	running|startup)
		URL="https://${HOST}/restconf/data${PATH}"
		;;
	operational)
		URL="https://${HOST}/restconf/data${PATH}"
		;;
	*)
		echo "Error: Invalid datastore '$DATASTORE'. Use: running, operational, or startup" >&2
		exit 1
		;;
esac

# Execute curl with all remaining arguments passed through
exec /usr/bin/curl \
	--insecure \
	--user "${AUTH}" \
	--request "${METHOD}" \
	--header "Content-Type: application/yang-data+json" \
	--header "Accept: application/yang-data+json" \
	"$@" \
	"${URL}"
