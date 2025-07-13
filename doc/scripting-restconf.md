# Examples using RESTCONF

## Factory Reset

```
~$ curl -kX POST -u admin:admin \
        -H "Content-Type: application/yang-data+json" \
        https://example.local/restconf/operations/ietf-factory-default:factory-reset
curl: (56) OpenSSL SSL_read: error:0A000126:SSL routines::unexpected eof while reading, errno 0
```

## System Reboot

```
~$ curl -kX POST -u admin:admin \
        -H "Content-Type: application/yang-data+json" \
        https://example.local/restconf/operations/ietf-system:system-restart
```

## Set Date and Time

Here's an example of an RPC that takes input/argument:

```
~$ curl -kX POST -u admin:admin \
        -H "Content-Type: application/yang-data+json" \
        -d '{"ietf-system:input": {"current-datetime": "2024-04-17T13:48:02-01:00"}}' \
        https://example.local/restconf/operations/ietf-system:set-current-datetime
```

You can verify that the changes took by a remote SSH command:

```
~$ ssh admin@example.local 'date'
Wed Apr 17 14:48:12 UTC 2024
~$
```

## Read Hostname

Example of fetching JSON configuration data to stdout:

```
~$ curl -kX GET -u admin:admin \
        -H 'Accept: application/yang-data+json' \
        https://example.local/restconf/data/ietf-system:system/hostname
{
  "ietf-system:system": {
    "hostname": "foo"
  }
}
```

## Set Hostname

Example of inline JSON data:

```
~$ curl -kX PATCH -u admin:admin \
     -H 'Content-Type: application/yang-data+json' \
     -d '{"ietf-system:system":{"hostname":"bar"}}' \
     https://example.local/restconf/data/ietf-system:system
```

## Copy Running to Startup

No copy command available yet to copy between datastores, and the
Rousette back-end also does not support "write-through" to the
startup datastore.

To save running-config to startup-config, use the following example to
fetch running to a local file and then update startup with it:

```
~$ curl -kX GET -u admin:admin -o running-config.json \
        -H 'Accept: application/yang-data+json'       \
         https://example.local/restconf/ds/ietf-datastores:running

~$ curl -kX PUT -u admin:admin -d @running-config.json \
        -H 'Content-Type: application/yang-data+json'  \
        https://example.local/restconf/ds/ietf-datastores:startup
```
