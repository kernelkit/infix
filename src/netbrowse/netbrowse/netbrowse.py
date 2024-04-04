#!/usr/bin/env python3
"""
   Main server loop.
"""
import sys

from flask       import Flask, render_template
from avahi_alias import AvahiAlias
from mdns_hosts  import MdnsHosts

app = Flask(__name__)

@app.route('/browse')
def browse():
    """The /browse network.local application"""
    mdns_hosts = MdnsHosts()
    hosts_services = mdns_hosts.scan()

    order = { 'HTTPS': 1, 'HTTP': 2, 'SSH': 3, 'SFTP': 4 }
    for _, details in hosts_services.items():
        details['services'].sort(key=lambda x: order.get(x['type'], 999))

    return render_template('browse.html', hosts_services=hosts_services)

def main():
    """Main entrypoint, advertises aliases from command line and starts browser."""
    avahi_aliases = AvahiAlias()
    for each in sys.argv[1:]:
        avahi_aliases.publish_cname(str(each).encode("utf-8", "strict"))

    try:
        app.run(debug=True)
    except KeyboardInterrupt:
        print("Exiting")

if __name__ == '__main__':
    main()
