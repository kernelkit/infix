#!/bin/bash
DEVICE="172.31.31.142"
AUTH="admin:admin"

echo "=== RESTCONF Notification Capabilities ==="
echo

echo "1. Available streams:"
curl -sk https://$DEVICE/restconf/data/ietf-restconf-monitoring:restconf-state/streams \
  -u $AUTH | jq -r '.["ietf-restconf-monitoring:restconf-state"].streams.stream[]?.name' 2>/dev/null || echo "None found"
echo

echo "2. Testing NETCONF stream (5 sec timeout):"
timeout 5 curl -sk -N https://$DEVICE/restconf/streams/NETCONF \
  -H "Accept: text/event-stream" -u $AUTH 2>&1 | head -10
echo

echo "3. Testing yang-push stream (5 sec timeout):"
timeout 5 curl -sk -N https://$DEVICE/restconf/streams/yang-push \
  -H "Accept: text/event-stream" -u $AUTH 2>&1 | head -10
echo

echo "4. Current ietf-system state:"
curl -sk https://$DEVICE/restconf/data/ietf-system:system \
  -u $AUTH | jq 2>/dev/null || echo "Failed"
