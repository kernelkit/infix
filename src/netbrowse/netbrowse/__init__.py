#!/usr/bin/env python3
"""
   Very basic mDNS scanner with HTML table renderer
"""
from flask import Flask, render_template
from .mdns_hosts import MdnsHosts

__all__ = ['MdnsHosts']

app = Flask(__name__)


@app.route('/')
@app.route('/netbrowse')
def index():
    """The /browse or network.local application"""
    mdns_hosts = MdnsHosts()
    hosts_services = mdns_hosts.scan()

    order = {'HTTPS': 1, 'HTTP': 2, 'SSH': 3, 'SFTP': 4}
    for _, details in hosts_services.items():
        details['services'].sort(key=lambda x: order.get(x['type'], 999))

    return render_template('browse.html', hosts_services=hosts_services)


def main():
    """Stand-alone running with Flask development server."""
    try:
        app.run(debug=True)
    except KeyboardInterrupt:
        print("Exiting")


if __name__ == '__main__':
    main()
