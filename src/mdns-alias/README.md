mDNS Alias
==========

This Python program both advertises CNAMEs (using D-Bus to Avahi).

To start the program:

    mdns-alias $hostname.local network.local

The latter CNAME can be used with nginx and netbrowse to provide
a basic mDNS service browser.
