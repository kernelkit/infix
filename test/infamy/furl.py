"""Simple HTTP URL fetcher and content checker

This module provides the a lightweight wrapper around urllib for testing
HTTP endpoints.  It's designed specifically for test scenarios where you
need to:

- Fetch HTTP content and verify it contains expected strings
- Perform checks from within network namespaces
- Validate multiple content patterns in a single request
- Handle network errors gracefully in test environments

Example usage:
    # Single needle check
    url = Furl("http://example.com/api")
    if url.check("success"):
        print("API returned success")

    # Multiple needle check (all must match)
    if url.check(["user: alice", "status: active", "role: admin"]):
        print("All user details found")

    # Check from network namespace
    with IsolatedMacVlan("eth0") as ns:
        if url.nscheck(ns, "expected content"):
            print("Content verified from namespace")
"""

import urllib.error
import urllib.parse
import urllib.request


class Furl:
    """Furl wraps urllib in a way similar to curl"""
    def __init__(self, url):
        """Create new URL checker"""
        self.url = urllib.parse.quote(url, safe='/:')

    def check(self, needles, timeout=10):
        """Connect to web server URL, fetch body and check for needle(s)

        Args:
            needles: String or list of strings to search for in response
            timeout: Request timeout in seconds

        Returns:
            bool: True if all needles found in response, False otherwise
        """
        # Backwards compat, make needles a list for uniform processing
        if isinstance(needles, str):
            needles = [needles]

        try:
            with urllib.request.urlopen(self.url, timeout=timeout) as response:
                text = response.read().decode('utf-8')
                return all(needle in text for needle in needles)
        except urllib.error.URLError:
            return False
        except ConnectionResetError:
            return False

    def nscheck(self, netns, needles):
        """"Call check() from netns

        Args:
            netns: Network namespace to call from
            needles: String or list of strings to search for in response

        Returns:
            bool: True if all needles found in response, False otherwise
        """
        return netns.call(lambda: self.check(needles))
