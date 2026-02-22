# System Management

The system utilizes YANG models for keeping configuration and operational
data. These databases can be managed through different interfaces such
as NETCONF, RESTCONF, and CLI via SSH or Console.

## SSH Management

An SSH server (SSHv2) is provided for remote management. It can be
enabled/disabled as shown below.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit ssh</b>
admin@example:/config/ssh/> <b>set enabled</b>
admin@example:/config/ssh/>
</code></pre>

By default the SSH server accepts connections to port 22 on all its IP
addresses, but this can be adjusted using the `listen` command. To
make the server (only) listen for incoming connections to IP address
_192.168.1.1_ and port _12345_ the following commands can be used.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit ssh</b>
admin@example:/config/ssh/> <b>show</b>
enabled true;
hostkey genkey;
listen ipv4 {
  address 0.0.0.0;
  port 22;
}
listen ipv6 {
  address ::;
  port 22;
}
admin@example:/config/ssh/> <b>no listen ipv6</b>
admin@example:/config/ssh/> <b>edit listen ipv4</b>
admin@example:/config/ssh/listen/ipv4/> <b>set address 192.168.1.1</b>
admin@example:/config/ssh/listen/ipv4/> <b>set port 12345</b>
admin@example:/config/ssh/listen/ipv4/>
</code></pre>

The default SSH hostkey is generated on first boot and is used in both
SSH and NETCONF (SSH transport). Custom keys can be added to the
configuration in `ietf-keystore`. The only supported hostkey type is
RSA for now, thus the private key must be
`ietf-crypto-types:rsa-private-key-format` and the public key
`ietf-crypto-types:ssh-public-key-format`

> [!TIP]
> For comprehensive information about the keystore, including key management,
> security considerations, and examples for different key types, see the
> [Keystore documentation](keystore.md).

### Use your own SSH hostkeys

Hostkeys can be generated with OpenSSL:
```bash
openssl genpkey -quiet -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -outform PEM > mykey
openssl rsa -RSAPublicKey_out < mykey > mykey.pub
```
Store the keys in `ietf-keystore` _without_ the header and footer information
created by OpenSSL.

After the key has been stored in the keystore and given the name
_mykey_ it can be added to SSH configuration:

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit ssh</b>
admin@example:/config/ssh/> <b>set hostkey mykey</b>
</code></pre>

## Console Port

For units with a console port, it is possible for users to login to
shell/CLI with functionality similar to what is provided via SSH.

The type and setup for your console port is product specific. For
instance, it can be a USB-C port connected to the CPU serial port
using a USB-to-serial converter. To connect you would need a USB-C cable
connected to the console port of the device. The serial port is
typically setup to run at 115200 baud, 8N1.


<pre class="cli"><code>Infix OS — Immutable.Friendly.Secure v24.11.1 (ttyS0)
example login: <b>admin</b>
Password:
.-------.
|  . .  | Infix OS — Immutable.Friendly.Secure
|-. v .-| https://kernelkit.org
'-'---'-'

Run the command 'cli' for interactive OAM

admin@example:~$
</code></pre>

The `resize` command can be used to update terminal settings to the
size of your terminal window.

<pre class="cli"><code>admin@example:~$ <b>resize</b>
COLUMNS=115;LINES=59;export COLUMNS LINES;
admin@example:~$
</code></pre>

CLI can be entered from shell in the same way as for SSH.

<pre class="cli"><code>admin@example:~$ <b>cli</b>

See the 'help' command for an introduction to the system

admin@example:/> <b>show interfaces</b>
<span class="header">INTERFACE       PROTOCOL   STATE       DATA                                    </span>
lo              ethernet   UP          00:00:00:00:00:00
                ipv4                   127.0.0.1/8 (static)
                ipv6                   ::1/128 (static)
e1              ethernet   LOWER-DOWN  00:53:00:06:03:01
e2              ethernet   LOWER-DOWN  00:53:00:06:03:02
...
admin@example:/>
</code></pre>

## Web, Web-console and RESTCONF

The system provides a set of Web services:

- a rudimentary Web server, currently limited to an information page
- a RESTCONF server with equivalent management capabilities as NETCONF
- a Web console service, where the shell/CLI can be accessed via
  HTTPS, similar to connecting via a console port or SSH

There is also a *Netbrowse* Web service presenting information about
the unit's neighbors, collected via mDNS (see
[Discovery](discovery.md) for more details).

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit web</b>
admin@example:/config/web/> <b>help</b>
  certificate                       Reference to asymmetric key in central keystore.
  enabled                           Enable or disable on all web services.
  console                           Web console interface.
  netbrowse                         mDNS Network Browser.
  restconf                          IETF RESTCONF Server.
admin@example:/config/web/>
</code></pre>

### Enable/disable Web Service and Server

The Web service can be enabled as shown below.

<pre class="cli"><code>admin@example:/> <b>configure</b>
admin@example:/config/> <b>edit web</b>
admin@example:/config/web/> <b>set enabled</b>
admin@example:/config/web/>
</code></pre>

Enabling the Web service implies that a Web server is
enabled. Currently this Web server provides generic Infix information,
as well as a link to a Web console. The Web server uses HTTPS; any
HTTP request is redirected to HTTPS.

The _enabled_ setting for the Web service acts as a global
enable/disable setting for the other Web services (Web console,
RESTCONF and Netbrowse).

### Enable/disable Web Console

The Web console service provides a terminal service similar to Console
or SSH. The Web console is secured via HTTPS on port 7861.

The Web console has its own enable/disable setting, but will only be
activated if the Web service is enabled. The example below shows how
to disable the Web console.

<pre class="cli"><code>admin@example:/config/web/> <b>edit console</b>
admin@example:/config/web/console/> <b>no enabled</b>
admin@example:/config/web/console/>
</code></pre>

### Enable/disable RESTCONF Service

Alternatively, the system can be managed remotely using
RESTCONF. Meaning you can `curl` it instead of using a dedicated
NETCONF client.

The RESTCONF service has its own enable/disable setting, but will
only be activated if the Web service is enabled. The example below
shows how to disable the RESTCONF service.

<pre class="cli"><code>admin@example:/config/web/> <b>edit restconf</b>
admin@example:/config/web/restconf/> <b>no enabled</b>
admin@example:/config/web/restconf/>
</code></pre>

### HTTPS Certificate

The Web server uses a TLS certificate from the central
[keystore](keystore.md).  By default it uses `gencert`, a self-signed
certificate that is automatically generated on first boot.

To use a different certificate, e.g., one signed by a CA, first add
it to the keystore as an asymmetric key with `x509-public-key-format`,
then point the web `certificate` leaf to it:

<pre class="cli"><code>admin@example:/config/web/> <b>set certificate my-cert</b>
admin@example:/config/web/>
</code></pre>

See [Keystore](keystore.md#tls-certificates) for details on managing
TLS certificates.

## System Upgrade

See [Upgrade & Boot Order](upgrade.md) for information on upgrading.
