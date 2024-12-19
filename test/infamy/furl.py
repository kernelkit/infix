"""Fugly URL fetcher"""

import urllib.error
import urllib.request

class Furl:
    """Furl wraps urllib in a way similar to curl"""
    def __init__(self, url):
        """Create new URL checker"""
        self.url = urllib.parse.quote(url, safe='/:')

    def check(self, needle, timeout=10):
        """Connect to web server URL, fetch body and check for needle"""
        try:
            with urllib.request.urlopen(self.url, timeout=timeout) as response:
                text = response.read().decode('utf-8')
                #print(text)
                return needle in text
        except urllib.error.URLError as _:
            return False
        except ConnectionResetError:
            return False

    def nscheck(self, netns, needle):
        """"Call check() from netns"""
        return netns.call(lambda: self.check(needle))
