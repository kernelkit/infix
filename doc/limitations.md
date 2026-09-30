# Limitations

Known limits of the system that are not obvious from the CLI or the
YANG models.  Each entry names the release that introduced it.

## Configuration String Limits

Several settings end up verbatim in configuration files of the daemons
that implement them, so their character set is restricted, since
v26.09.0.  A `startup-config` with a value outside these limits fails
validation on boot, and the system falls back to `failure-config`, see
[Broken startup-config](boot.md#broken-startup-config).  Check the
settings below before upgrading from an older release.

| Setting                                   | Allowed characters                                      | Length |
|-------------------------------------------|---------------------------------------------------------|--------|
| Interface `name`                          | Letters, digits, `_`, `.`, `:`, `+`, `-`, must start with a letter, digit, or `_` | |
| Hardware component `name`                 | As interface name, `@` also allowed                     |        |
| Keystore asymmetric key and certificate `name` | As interface name, `@` also allowed                |        |
| DHCP client option `value`                | Letters, digits, space, `_`, `.`, `:`, `/`, `@`, `=`, `,`, `+`, `-` | 1..255 |
| DHCP server static-host `hostname`        | Letters, digits, `_`, `.`, `-`                          | 1..255 |
| DHCP server static-host `client-id`       | Letters, digits, `_`, `.`, `:`, `+`, `-`                | 1..255 |
| Syslog `property-filter` value            | Letters, digits, space, `_`, `.`, `:`, `/`, `@`, `=`, `,`, `-`, and `+ * ? \| ^ $ ( ) [ ] { }` | |
| Syslog file and remote `pattern-match`    | As `property-filter` value                              |        |
| Wi-Fi `mesh-id`                           | Anything except control characters, `"`, and `\`        |        |
| Wi-Fi `nas-identifier`                    | Letters, digits, `_`, `.`, `:`, `+`, `-`                |        |
| Wi-Fi access point and mesh point passphrase | Printable characters, same rule as for station       | 8..63  |
| SNMP community and security names         | Anything except line breaks                             |        |

The exact patterns are in the YANG models, e.g., `infix-interfaces.yang`
for interface names.  A value is rejected at `leave` in the CLI, or by
the edit operation over NETCONF and RESTCONF, with the pattern that
failed.
