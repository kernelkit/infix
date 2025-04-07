# -*- sh -*-
# Modules from libnetconf2
# INFO: CHANGED FEATURE FLAGS FROM ORIGINAL: ietf-keystore and ietf-ssh-server

MODULES=(
	"iana-ssh-encryption-algs@2022-06-16.yang"
	"iana-ssh-key-exchange-algs@2022-06-16.yang"
	"iana-ssh-mac-algs@2022-06-16.yang"
	"iana-ssh-public-key-algs@2022-06-16.yang"
	"iana-tls-cipher-suite-algs@2022-06-16.yang"
	"ietf-x509-cert-to-name@2014-12-10.yang"
	"iana-crypt-hash@2014-04-04.yang -e crypt-hash-md5 -e crypt-hash-sha-256 -e crypt-hash-sha-512"
	"ietf-crypto-types@2023-12-28.yang -e cleartext-passwords -e cleartext-private-keys"
	"ietf-keystore@2023-12-28.yang -e central-keystore-supported -e asymmetric-keys"
	"ietf-truststore@2023-12-28.yang -e central-truststore-supported -e certificates -e public-keys"
	"ietf-tcp-common@2023-12-28.yang -e keepalives-supported"
	"ietf-tcp-server@2023-12-28.yang -e tcp-server-keepalives"
	"ietf-tcp-client@2023-12-28.yang -e local-binding-supported -e tcp-client-keepalives"
	"ietf-ssh-common@2023-12-28.yang -e transport-params"
	"ietf-ssh-server@2023-12-28.yang"
	"ietf-tls-common@2023-12-28.yang -e tls10 -e tls11 -e tls12 -e tls13 -e hello-params"
	"ietf-tls-server@2023-12-28.yang -e server-ident-x509-cert -e client-auth-supported -e client-auth-x509-cert"
	"ietf-netconf-server@2023-12-28.yang -e ssh-listen -e tls-listen -e ssh-call-home -e tls-call-home -e central-netconf-server-supported"
	"libnetconf2-netconf-server@2024-07-09.yang"
)
