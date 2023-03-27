Infix System Configuration and RESTCONF API
===========================================

This document shows how to configure settings in the Infix CLI and how
to remotely inspect and change the configuration, as well as send RPC
commands, using the RESTCONF API.


Configuration
-------------


### NTP Client

How to configure two NTP servers with different options:

```
set system ntp 
set system ntp server default 
set system ntp server default udp address 1.2.3.4
set system ntp server default udp port 1231
set system ntp server default iburst true
set system ntp server default prefer true
set system ntp server foo 
set system ntp server foo udp address 4.3.2.1
set system ntp server foo iburst false
commit
```

How to set up an actually working configuration when the system has a
valid IP address:

```
set system ntp server bar
set system ntp server bar udp address 192.168.122.1
commit
```


### Static IP Address

DHCP support not yet ready, but would be the default in a product
with NETCONF as the primary interface.  This eliminates the need
to set up the device and give a works-out-of-the-box experience.
(Pending separate discussion on security aspects, of course).

```
set interface eth0 
set interface eth0 type ianaift:ethernetCsmacd
set interface eth0 enabled true
set interface eth0 ipv4 
set interface eth0 ipv4 enabled true
set interface eth0 ipv4 address 192.168.122.22 
set interface eth0 ipv4 address 192.168.122.22 prefix-length 24
commit
```

Multiple IP addresses per interface will be in the final product.

```
> show configuration
ietf-system:system {
   hostname flood;
}
ietf-interfaces:interfaces {
   interface eth0 {
      type ianaift:ethernetCsmacd;
      enabled true;
      ietf-ip:ipv4 {
         enabled true;
         address 192.168.122.22 {
            prefix-length 24;
         }
      }
   }
   interface eth1 {
   }
}
clixon-restconf:restconf {
   enable true;
   auth-type none;
   fcgi-socket /run/clixon/restconf.sock;
}
```

**Note:** authentication disabled for demo purposes.

To save the configuration so that it persists across reboots:

```
copy running startup
```


RESTCONF API
------------

Get a subset of the configuration:

```
$ curl -X GET http://192.168.122.22/restconf/data/ietf-system:system
{
   "ietf-system:system": {
      "hostname": "foo"
   }
}
```

```
$ curl -k -X GET http://192.168.122.22/restconf/data/ietf-system:system-state/
{
   "ietf-system:system-state": {
      "platform": {
         "os-name": "Infix",
         "os-release": "Buildroot 2023.02",
         "os-version": "latest-227-g51c35e9-dirty",
         "machine": "x86_64"
      },
      "clock": {
         "current-datetime": "2023-03-23T22:15:53+01:00",
         "boot-datetime": "2023-03-23T22:08:42+01:00"
      }
   }
}
```


List available RPC operations:

```
$ curl -X GET http://192.168.122.22/restconf/operations
{"operations": {
"ietf-system:set-current-datetime": [null],
	"ietf-system:system-restart": [null],
	"ietf-system:system-shutdown": [null],
	"clixon-lib:debug": [null],
	"clixon-lib:ping": [null],
	"clixon-lib:stats": [null],
	"clixon-lib:restart-plugin": [null],
	"clixon-lib:process-control": [null],
	"ietf-netconf-monitoring:get-schema": [null],
	"ietf-netconf:get-config": [null],
	"ietf-netconf:edit-config": [null],
	"ietf-netconf:copy-config": [null],
	"ietf-netconf:delete-config": [null],
	"ietf-netconf:lock": [null],
	"ietf-netconf:unlock": [null],
	"ietf-netconf:get": [null],
	"ietf-netconf:close-session": [null],
	"ietf-netconf:kill-session": [null],
	"ietf-netconf:commit": [null],
	"ietf-netconf:discard-changes": [null],
	"ietf-netconf:cancel-commit": [null],
	"ietf-netconf:validate": [null],
	"clixon-rfc5277:create-subscription": [null],
	"ietf-netconf-nmda:get-data": [null],
	"ietf-netconf-nmda:edit-data": [null]}
}
```


### Set hostname remotely

```
$ curl -is -H 'Content-Type: application/yang-data+json' -H 'Accept: application/yang-data+json' -X PATCH -d @hostname.cfg http://192.168.122.22/restconf/data/ietf-system:system
HTTP/1.1 204 No Content
Server: nginx/1.22.1
Date: Thu, 16 Mar 2023 13:53:53 GMT
Connection: keep-alive
```

The file `hostname.cfg` is in JSON format and looks like this:

```
{
   "ietf-system:system": {
      "hostname": "foo"
   }
}
```

One-liner command without external `.cfg` file:

```
$ curl -is -H 'Content-Type: application/yang-data+json' -H 'Accept: application/yang-data+json' -X PATCH -d '{"ietf-system:system":{"hostname":"flood"}}' http://192.168.122.22/restconf/data/ietf-system:system
HTTP/1.1 204 No Content
Server: nginx/1.22.1
Date: Thu, 16 Mar 2023 14:1
```

Run this on a PC connected to the "switch" to see how the settings take effect immediately:

```
$ watch 'lldpcli show neighbors |grep -A20 tap0'
```

### RPCs

Perform system reboot remotely:

```
$ curl -k -X POST http://192.168.122.22/restconf/operations/ietf-system:system-restart
```

Enable debug remotely:

```
$ curl -Ssik -X POST -H "Content-Type: application/yang-data+json" http://192.168.122.22/restconf/operations/clixon-lib:debug -d '{"clixon-lib:input":{"level":1}}'
```

Set current date/time:

```
$ curl -Ssik -X POST -H "Content-Type: application/yang-data+json" http://192.168.122.22/restconf/operations/ietf-system:set-current-datetime -d '{"ietf-system:input":{"current-datetime":"2023-03-23T22:15:53+01:00"}}'
HTTP/1.1 204 No Content
Server: nginx/1.22.1
Date: Thu, 23 Mar 2023 22:46:52 GMT
Connection: keep-alive
```
$ curl -Ssik -X POST -H "Content-Type: application/yang-data+json" http://192.168.122.22/restconf/operations/ietf-system:set-current-datetime -d '{"ietf-system:input":{"current-datetime":"2023-03-23T20:15:53+05:00"}}'

