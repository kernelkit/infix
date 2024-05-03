/* SPDX-License-Identifier: ISC */

#include <assert.h>
#include <errno.h>
#include <getopt.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <openssl/pem.h>
#include <openssl/x509.h>
#include <openssl/x509v3.h>
#include <openssl/rand.h>
#include <openssl/evp.h>
#include <openssl/bn.h>

extern char *__progname;

static int usage(int rc)
{
	fprintf(stderr,
		"Usage: %s\n"
		"\n"
		"Options:\n"
		"  --country COUNTRY         Set country, default: US\n"
		"  --state STATE             Set state or province, default: California\n"
		"  --city CITY               Set city name, default: Berkeley\n"
		"  --organization ORG        Set organization name, default: Acme, Inc.\n"
		"  --organization-unit UNIT  Set organization unit name, default: Second\n"
		"  --common-name CN          Set common name, default: none (required!)\n"
		"  --out-certificate FILE    Output certificate\n"
		"  --out-key FILE            Output private key\n"
		, __progname);
	return rc;
}

int main(int argc, char *argv[])
{
	struct option opts[] = {
		{"city",              required_argument, 0, 'l'},
		{"common-name",       required_argument, 0, 'n'},
		{"country",           required_argument, 0, 'c'},
		{"help",              no_argument,       0, 'h'},
		{"organization",      required_argument, 0, 'o'},
		{"organisation",      required_argument, 0, 'o'},
		{"organization-unit", required_argument, 0, 'u'},
		{"organisation-unit", required_argument, 0, 'u'},
		{"out-certificate",   required_argument, 0, 'p'},
		{"out-key",           required_argument, 0, 'k'},
		{"state",             required_argument, 0, 's'},
		{0, 0, 0, 0} 
	};
	char *common_name = NULL, *cert = "certificate.pem", *key = "private.key";
	char *country = "US", *state = "California", *locality = "Berkeley";
	char *organization = "Acme, Inc.", *unit = "Second";
	ASN1_INTEGER *asn1_serno;
	EVP_PKEY_CTX *pkey_ctx;
	BIGNUM *bn_serial;
	X509_NAME *name;
	EVP_PKEY *pkey;
	X509 *x509;
	FILE *fp;
	int c;

	while ((c = getopt_long(argc, argv, "c:hs:l:o:u:n:", opts, NULL)) != -1) {
		switch (c) {
		case 'c':
			country = optarg;
			break;
		case 'h':
		case '?':
			return usage(0);
		case 'k':
			key = optarg;
			break;
		case 'l':
			locality = optarg;
			break;
		case 'n':
			common_name = optarg;
			break;
		case 'o':
			organization = optarg;
			break;
		case 'p':
			cert = optarg;
			break;
		case 's':
			state = optarg;
			break;
		case 'u':
			unit = optarg;
			break;
		default:
			fprintf(stderr, "got here: '%c'\n", c);
			abort();
		}
	}

	if (!common_name)
		return usage(1);

	OpenSSL_add_all_algorithms();

	/* Key generation using EVP */
	pkey = EVP_PKEY_new();
	pkey_ctx = EVP_PKEY_CTX_new_id(EVP_PKEY_RSA, NULL);
	if (!EVP_PKEY_keygen_init(pkey_ctx) || !EVP_PKEY_CTX_set_rsa_keygen_bits(pkey_ctx, 2048) ||
	    !EVP_PKEY_keygen(pkey_ctx, &pkey)) {
		fprintf(stderr, "Error generating RSA key\n");
		EVP_PKEY_CTX_free(pkey_ctx);
		return 1;
	}
	EVP_PKEY_CTX_free(pkey_ctx);

	/* Certificate creation */
	x509 = X509_new();
	assert(x509);
	X509_set_version(x509, 2);

	/* Generating a random serial number */
	bn_serial = BN_new();
	assert(bn_serial);
	BN_rand(bn_serial, 160, 0, 0);  /* Generate a 160-bit serial number */
	asn1_serno = BN_to_ASN1_INTEGER(bn_serial, NULL);
	assert(asn1_serno);
	X509_set_serialNumber(x509, asn1_serno);

	ASN1_TIME_set_string(X509_getm_notBefore(x509), "20000101000000Z");
	ASN1_TIME_set_string(X509_getm_notAfter(x509), "99990101000000Z");

	name = X509_get_subject_name(x509);
	X509_NAME_add_entry_by_txt(name, "C",  MBSTRING_ASC, (unsigned char *)country, -1, -1, 0);
	X509_NAME_add_entry_by_txt(name, "ST", MBSTRING_ASC, (unsigned char *)state, -1, -1, 0);
	X509_NAME_add_entry_by_txt(name, "L",  MBSTRING_ASC, (unsigned char *)locality, -1, -1, 0);
	X509_NAME_add_entry_by_txt(name, "O",  MBSTRING_ASC, (unsigned char *)organization, -1, -1, 0);
	X509_NAME_add_entry_by_txt(name, "OU", MBSTRING_ASC, (unsigned char *)unit, -1, -1, 0);
	X509_NAME_add_entry_by_txt(name, "CN", MBSTRING_ASC, (unsigned char *)common_name, -1, -1, 0);

	X509_set_issuer_name(x509, name);
	X509_set_pubkey(x509, pkey);

	if (!X509_sign(x509, pkey, EVP_sha256())) {
		fprintf(stderr, "Error signing certificate\n");
		return 1;
	}

	/* Saving the certificate */
	fp = fopen(cert, "w");
	if (!fp) {
		fprintf(stderr, "Failed opening %s for writing: %s\n", cert, strerror(errno));
		goto done;
	}
	PEM_write_X509(fp, x509);
	fclose(fp);

	/* Saving the private key */
	fp = fopen(key, "w");
	if (!fp) {
		fprintf(stderr, "Failed opening %s for writing: %s\n", key, strerror(errno));
		goto done;
	}
	PEM_write_PrivateKey(fp, pkey, NULL, NULL, 0, NULL, NULL);
	fclose(fp);

done:	/* Cleanup */
	X509_free(x509);
	EVP_PKEY_free(pkey);
	BN_free(bn_serial);
	ASN1_INTEGER_free(asn1_serno);

	return 0;
}
