# -*- sh -*-
# Modules from libnetconf2
# INFO: CHANGED FEATURE FLAGS FROM ORIGINAL: ietf-keystore
# NETCONF is an sshd subsystem, the SSH/TLS transport modules are not loaded

MODULES=(
	"iana-tls-cipher-suite-algs@2024-10-16.yang"
	"ietf-x509-cert-to-name@2014-12-10.yang"
	"iana-crypt-hash@2014-04-04.yang -e crypt-hash-md5 -e crypt-hash-sha-256 -e crypt-hash-sha-512"
	"ietf-crypto-types@2024-10-10.yang -e cleartext-passwords -e cleartext-private-keys"
	"ietf-keystore@2024-10-10.yang -e central-keystore-supported -e inline-definitions-supported -e asymmetric-keys"
	"ietf-truststore@2024-10-10.yang -e central-truststore-supported -e inline-definitions-supported -e certificates -e public-keys"
	"ietf-tcp-common@2024-10-10.yang -e keepalives-supported"
	"ietf-tcp-client@2024-10-10.yang -e local-binding-supported -e tcp-client-keepalives"
	"ietf-tls-common@2024-10-10.yang -e algorithm-discovery -e tls12 -e tls13 -e hello-params"
)
