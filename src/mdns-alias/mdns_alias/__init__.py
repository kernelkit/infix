"""
    Avahi CNAME service over D-Bus advertiser
"""
import sys
import time

from .mdns_alias import MdnsAlias
__all__ = ['MdnsAlias']

def main():
    """Advertises aliases from command line."""
    mdns_aliases = MdnsAlias()

    for arg in sys.argv[1:]:
        mdns_aliases.publish_cname(str(arg).encode("utf-8", "strict"))

    while True:
        time.sleep(3600)

if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        print("Exiting")
