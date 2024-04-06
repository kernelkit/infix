mDNS Network Browser
====================

This program is a Python Flask app that provides an mDNS browser for,
e.g., Nginx.  It is intended to answer calls to https://network.local

A UNIX socket, for fastcgi, is created in `/tmp/netbrowse.sock` with
permissions 0660 as the user and group the program is started as.

When using Finit this can be achieved with

    service @www-data:www-data netbrowse network.local

In your Nginx server configuration, add:

	location /browse {
			include fastcgi_params;
			fastcgi_pass unix:/tmp/netbrowse.sock;
	}

For more a elaborate setup, you can have another server block:

```
server {
    listen 80;
    listen [::]:80;
    server_name network.local;

    location / {
        include fastcgi_params;
        fastcgi_pass unix:/tmp/netbrowse.sock;
    }
}
```
