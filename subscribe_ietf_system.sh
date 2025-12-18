#!/bin/bash
# Simple bash script to subscribe to ietf-system notifications from rousette
# Uses curl to interact with RESTCONF API

set -euo pipefail

# Configuration
RESTCONF_URL="${RESTCONF_URL:-http://localhost:10080}"
USERNAME="${USERNAME:-}"
PASSWORD="${PASSWORD:-}"
XPATH_FILTER="${XPATH_FILTER:-/ietf-system:*}"
NO_VERIFY_SSL="${NO_VERIFY_SSL:-false}"

# Build authentication flags
AUTH_FLAGS=()
if [ -n "$USERNAME" ]; then
    AUTH_FLAGS=(-u "$USERNAME:$PASSWORD")
fi

# Add SSL flags for self-signed certificates
if [ "$NO_VERIFY_SSL" = "true" ]; then
    AUTH_FLAGS+=(-k)
    echo "Warning: SSL certificate verification disabled" >&2
fi

echo "=== Establishing RESTCONF subscription ===" >&2
echo "Server: $RESTCONF_URL" >&2
echo "Filter: $XPATH_FILTER" >&2
echo >&2

# Step 1: Establish subscription
SUBSCRIPTION_REQUEST=$(cat <<EOF
{
  "ietf-subscribed-notifications:input": {
    "stream": "NETCONF",
    "encoding": "encode-json",
    "stream-xpath-filter": "$XPATH_FILTER"
  }
}
EOF
)

SUBSCRIPTION_RESPONSE=$(curl -s "${AUTH_FLAGS[@]}" \
    -H "Content-Type: application/yang-data+json" \
    -H "Accept: application/yang-data+json" \
    -X POST \
    "$RESTCONF_URL/restconf/operations/ietf-subscribed-notifications:establish-subscription" \
    -d "$SUBSCRIPTION_REQUEST")

# Parse response (using jq if available, otherwise grep/sed)
if command -v jq &> /dev/null; then
    SUBSCRIPTION_ID=$(echo "$SUBSCRIPTION_RESPONSE" | jq -r '."ietf-subscribed-notifications:output".id')
    SUBSCRIPTION_URI=$(echo "$SUBSCRIPTION_RESPONSE" | jq -r '."ietf-subscribed-notifications:output"."ietf-restconf-subscribed-notifications:uri"')
else
    # Fallback parsing without jq
    SUBSCRIPTION_ID=$(echo "$SUBSCRIPTION_RESPONSE" | grep -o '"id":[0-9]*' | cut -d: -f2)
    SUBSCRIPTION_URI=$(echo "$SUBSCRIPTION_RESPONSE" | grep -o '"/streams/subscribed/[^"]*"' | tr -d '"')
fi

if [ -z "$SUBSCRIPTION_ID" ] || [ -z "$SUBSCRIPTION_URI" ]; then
    echo "Error: Failed to establish subscription" >&2
    echo "Response: $SUBSCRIPTION_RESPONSE" >&2
    exit 1
fi

echo "✓ Subscription established (ID: $SUBSCRIPTION_ID)" >&2
echo "  Stream URI: $SUBSCRIPTION_URI" >&2
echo >&2

# Step 2: Connect to notification stream
echo "=== Listening for notifications (Ctrl+C to stop) ===" >&2
echo >&2

curl "${AUTH_FLAGS[@]}" \
    -H "Accept: text/event-stream" \
    -N \
    "$RESTCONF_URL$SUBSCRIPTION_URI" | while IFS= read -r line; do
    # SSE format: lines starting with "data:" contain the actual notification
    if [[ $line == data:* ]]; then
        # Extract JSON data after "data:" prefix
        notification="${line#data:}"

        # Pretty-print with jq if available
        if command -v jq &> /dev/null; then
            echo "$notification" | jq '.'
        else
            echo "$notification"
        fi
        echo "----------------------------------------"
    fi
done
