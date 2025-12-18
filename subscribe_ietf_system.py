#!/usr/bin/env python3
"""
Subscribe to RESTCONF notifications for ietf-system changes from rousette.

This script establishes a subscription to the NETCONF notification stream,
filters for ietf-system notifications, and displays them in real-time.
"""

import argparse
import json
import re
import sys
from typing import Optional

import requests
from requests.auth import HTTPBasicAuth


class SSEClient:
    """Simple Server-Sent Events (SSE) client."""

    def __init__(self, response):
        self.response = response
        self.events = self._parse_events()

    def _parse_events(self):
        """Parse SSE stream."""
        data_buffer = []

        for line in self.response.iter_lines(decode_unicode=True):
            if line is None:
                continue

            # Empty line indicates end of event
            if not line.strip():
                if data_buffer:
                    yield '\n'.join(data_buffer)
                    data_buffer = []
                continue

            # SSE data line
            if line.startswith('data:'):
                data_buffer.append(line[5:].strip())
            # SSE comment (ignore)
            elif line.startswith(':'):
                continue


def establish_subscription(
    base_url: str,
    username: Optional[str] = None,
    password: Optional[str] = None,
    xpath_filter: Optional[str] = None,
    encoding: str = "encode-json",
    verify_ssl: bool = True
) -> dict:
    """
    Establish a RESTCONF notification subscription.

    Args:
        base_url: Base URL of the RESTCONF server (e.g., "http://localhost:10080")
        username: Username for authentication (None for anonymous)
        password: Password for authentication
        xpath_filter: XPath filter for notifications (e.g., "/ietf-system:*")
        encoding: Notification encoding ("encode-json" or "encode-xml")

    Returns:
        dict with 'id' and 'uri' keys
    """
    url = f"{base_url}/restconf/operations/ietf-subscribed-notifications:establish-subscription"

    # Build subscription request
    payload = {
        "ietf-subscribed-notifications:input": {
            "stream": "NETCONF",
            "encoding": encoding
        }
    }

    # Add XPath filter if provided
    if xpath_filter:
        payload["ietf-subscribed-notifications:input"]["stream-xpath-filter"] = xpath_filter

    headers = {
        "Content-Type": "application/yang-data+json",
        "Accept": "application/yang-data+json"
    }

    # Setup authentication
    auth = None
    if username:
        auth = HTTPBasicAuth(username, password or "")

    # Make subscription request
    print(f"Establishing subscription to {url}...")
    if xpath_filter:
        print(f"  XPath filter: {xpath_filter}")
    if not verify_ssl:
        print("  Warning: SSL certificate verification disabled")

    # Disable SSL warnings if verify_ssl is False
    if not verify_ssl:
        import urllib3
        urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

    response = requests.post(url, json=payload, headers=headers, auth=auth, verify=verify_ssl)

    if response.status_code != 200:
        print(f"Error: Failed to establish subscription (HTTP {response.status_code})")
        print(f"Response: {response.text}")
        sys.exit(1)

    # Parse response
    data = response.json()
    output = data.get("ietf-subscribed-notifications:output", {})

    subscription_id = output.get("id")
    subscription_uri = output.get("ietf-restconf-subscribed-notifications:uri")

    if not subscription_id or not subscription_uri:
        print("Error: Invalid subscription response")
        print(f"Response: {json.dumps(data, indent=2)}")
        sys.exit(1)

    print(f"✓ Subscription established (ID: {subscription_id})")
    print(f"  Stream URI: {subscription_uri}")

    return {
        "id": subscription_id,
        "uri": subscription_uri
    }


def receive_notifications(
    base_url: str,
    subscription_uri: str,
    username: Optional[str] = None,
    password: Optional[str] = None,
    pretty: bool = True,
    verify_ssl: bool = True
):
    """
    Connect to the subscription stream and receive notifications.

    Args:
        base_url: Base URL of the RESTCONF server
        subscription_uri: Subscription URI from establish_subscription
        username: Username for authentication
        password: Password for authentication
        pretty: Pretty-print JSON notifications
    """
    # Build full URL
    url = f"{base_url}{subscription_uri}"

    headers = {
        "Accept": "text/event-stream"
    }

    # Setup authentication
    auth = None
    if username:
        auth = HTTPBasicAuth(username, password or "")

    print(f"\nConnecting to notification stream...")
    print(f"Listening for ietf-system notifications (Ctrl+C to stop)...\n")

    try:
        # Disable SSL warnings if verify_ssl is False
        if not verify_ssl:
            import urllib3
            urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

        # Open SSE connection
        response = requests.get(url, headers=headers, auth=auth, stream=True, verify=verify_ssl)

        if response.status_code != 200:
            print(f"Error: Failed to connect to stream (HTTP {response.status_code})")
            print(f"Response: {response.text}")
            sys.exit(1)

        # Process events
        client = SSEClient(response)
        for event_data in client.events:
            try:
                # Parse and display notification
                if pretty:
                    data = json.loads(event_data)
                    print(json.dumps(data, indent=2))
                else:
                    print(event_data)
                print("-" * 80)
            except json.JSONDecodeError:
                print(f"Received non-JSON data: {event_data}")
                print("-" * 80)

    except KeyboardInterrupt:
        print("\n\nStopping notification listener...")
    except requests.exceptions.RequestException as e:
        print(f"\nConnection error: {e}")
        sys.exit(1)


def main():
    parser = argparse.ArgumentParser(
        description="Subscribe to RESTCONF notifications for ietf-system changes",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Subscribe with authentication
  %(prog)s --url http://localhost:10080 --user admin --password secret

  # Subscribe anonymously (if allowed by server)
  %(prog)s --url http://localhost:10080

  # Subscribe with custom XPath filter
  %(prog)s --url http://localhost:10080 --filter "/ietf-system:system-restart"

  # Subscribe to all notifications (no filter)
  %(prog)s --url http://localhost:10080 --no-filter
        """
    )

    parser.add_argument(
        "--url",
        default="http://localhost:10080",
        help="Base URL of RESTCONF server (default: http://localhost:10080)"
    )
    parser.add_argument(
        "--no-verify-ssl", "-k",
        action="store_true",
        help="Disable SSL certificate verification (for self-signed certificates)"
    )
    parser.add_argument(
        "--user", "-u",
        help="Username for authentication (omit for anonymous access)"
    )
    parser.add_argument(
        "--password", "-p",
        help="Password for authentication"
    )
    parser.add_argument(
        "--filter",
        default="/ietf-system:*",
        help="XPath filter for notifications (default: /ietf-system:*)"
    )
    parser.add_argument(
        "--no-filter",
        action="store_true",
        help="Don't apply any filter (receive all notifications)"
    )
    parser.add_argument(
        "--encoding",
        choices=["encode-json", "encode-xml"],
        default="encode-json",
        help="Notification encoding format (default: encode-json)"
    )
    parser.add_argument(
        "--plain",
        action="store_true",
        help="Don't pretty-print JSON output"
    )

    args = parser.parse_args()

    # Determine filter
    xpath_filter = None if args.no_filter else args.filter

    # Establish subscription
    subscription = establish_subscription(
        base_url=args.url,
        username=args.user,
        password=args.password,
        xpath_filter=xpath_filter,
        encoding=args.encoding,
        verify_ssl=not args.no_verify_ssl
    )

    # Receive notifications
    receive_notifications(
        base_url=args.url,
        subscription_uri=subscription["uri"],
        username=args.user,
        password=args.password,
        pretty=not args.plain,
        verify_ssl=not args.no_verify_ssl
    )


if __name__ == "__main__":
    main()
