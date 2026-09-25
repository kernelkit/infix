# -*- sh -*-
# SSH and TLS transports of libnetconf2, when netopeer2-server owns NETCONF
# itself instead of running as an OpenSSH subsystem.

MODULES=(
	"iana-ssh-encryption-algs@2024-10-16.yang"
	"iana-ssh-key-exchange-algs@2024-10-16.yang"
	"iana-ssh-mac-algs@2024-10-16.yang"
	"iana-ssh-public-key-algs@2024-10-16.yang"
	"ietf-tcp-server@2024-10-10.yang -e tcp-server-keepalives"
	"ietf-ssh-common@2024-10-10.yang -e algorithm-discovery -e transport-params"
	"ietf-ssh-server@2024-10-10.yang"
	"ietf-tls-server@2024-10-10.yang -e server-ident-x509-cert -e client-auth-supported -e client-auth-x509-cert"
	"ietf-netconf-server@2025-04-24.yang -e ssh-listen -e tls-listen -e ssh-call-home -e tls-call-home -e central-netconf-server-supported"
	"libnetconf2-netconf-server@2025-11-11.yang"
)
