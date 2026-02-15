/* SPDX-License-Identifier: BSD-3-Clause */

#include <sys/stat.h>
#include <srx/common.h>
#include <srx/lyx.h>

#include <srx/srx_val.h>

#include "base64.h"
#include "core.h"
#include "interfaces.h"

#define XPATH_KEYSTORE_ASYM "/ietf-keystore:keystore/asymmetric-keys"
#define XPATH_KEYSTORE_SYM  "/ietf-keystore:keystore/symmetric-keys"
#define SSH_PRIVATE_KEY  "/tmp/ssh.key"
#define SSH_PUBLIC_KEY   "/tmp/ssh.pub"
#define TLS_TMPDIR       "/tmp/ssl"
#define TLS_PRIVATE_KEY  TLS_TMPDIR "/self-signed.key"
#define TLS_CERTIFICATE  TLS_TMPDIR "/self-signed.crt"

/* return file size */
static size_t filesz(const char *fn)
{
	struct stat st;

	if (stat(fn, &st))
		st.st_size = BUFSIZ;
	return st.st_size;
}

/* read file of max len bytes, return as malloc()'ed buffer */
static char *filerd(const char *fn, size_t len)
{
	char *buf, *ptr;
	FILE *pp;

	ptr = buf = malloc(len + 1);
	if (!buf)
		return NULL;

	/* strip ---BEGIN//---END markers and concatenate lines */
	pp = popenf("r", "grep -v -- '-----' %s | tr -d '\n'", fn);
	if (!pp) {
		free(buf);
		return NULL;
	}

	while (fgets(ptr, len, pp)) {
		size_t inc = strlen(chomp(ptr));

		ptr += inc;
		len -= inc;
	}
	pclose(pp);

	return buf;
}

static int gen_hostkey(const char *name, struct lyd_node *change)
{
	const char *private_key, *public_key;
	int rc = SR_ERR_OK;

	private_key = lydx_get_cattr(change, "cleartext-private-key");
	public_key = lydx_get_cattr(change, "public-key");

	/* Validate keys before use */
	if (!private_key || !public_key || !*private_key || !*public_key)
		return SR_ERR_OK;

	if (mkdir(SSH_HOSTKEYS_NEXT, 0600) && (errno != EEXIST)) {
		ERRNO("Failed creating %s", SSH_HOSTKEYS_NEXT);
		rc = SR_ERR_INTERNAL;
	}

	AUDIT("Installing SSH host key \"%s\".", name);
	if (systemf("/usr/libexec/infix/mksshkey %s %s %s %s", name,
		    SSH_HOSTKEYS_NEXT, public_key, private_key))
		rc = SR_ERR_INTERNAL;

	return rc;
}

static int gen_webcert(const char *name, struct lyd_node *change)
{
	const char *private_key, *cert_data, *certname;
	struct lyd_node *certs, *cert;
	FILE *fp;

	erase("/run/finit/cond/usr/mkcert");

	private_key = lydx_get_cattr(change, "cleartext-private-key");
	if (!private_key || !*private_key) {
		ERROR("Cannot find private key for \"%s\"", name);
		return SR_ERR_OK;
	}

	certs = lydx_get_descendant(lyd_child(change), "certificates", "certificate", NULL);
	if (!certs) {
		ERROR("Cannot find any certificates for \"%s\"", name);
		return SR_ERR_OK;
	}

	cert = certs;		/* Use first certificate */

	certname = lydx_get_cattr(cert, "name");
	if (!certname || !*certname) {
		ERROR("Cannot find certificate name for \"%s\"", name);
		return SR_ERR_OK;
	}

	cert_data = lydx_get_cattr(cert, "cert-data");
	if (!cert_data || !*cert_data) {
		ERROR("Cannot find certificate data \"%s\"", name);
		return SR_ERR_OK;
	}

	AUDIT("Installing HTTPS %s certificate \"%s\"", name, certname);
	fp = fopenf("w", "%s/%s.key", SSL_KEY_DIR, certname);
	if (!fp) {
		ERRNO("Failed creating key file for \"%s\"", certname);
		return SR_ERR_INTERNAL;
	}
	fprintf(fp, "-----BEGIN RSA PRIVATE KEY-----\n%s\n-----END RSA PRIVATE KEY-----\n", private_key);
	fclose(fp);
	systemf("chmod 600 %s/%s.key", SSL_KEY_DIR, certname);

	fp = fopenf("w", "%s/%s.crt", SSL_CERT_DIR, certname);
	if (!fp) {
		ERRNO("Failed creating crt file for \"%s\"", certname);
		return SR_ERR_INTERNAL;
	}
	fprintf(fp, "-----BEGIN CERTIFICATE-----\n%s\n-----END CERTIFICATE-----\n", cert_data);
	fclose(fp);

	symlink("/run/finit/cond/reconf", "/run/finit/cond/usr/mkcert");

	return SR_ERR_OK;
}

static int keystore_update(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff)
{
	const char *xpath = "/ietf-keystore:keystore/asymmetric-keys/asymmetric-key";
	sr_val_t *list = NULL;
        size_t count = 0;
	int rc;

	rc = sr_get_items(session, xpath, 0, 0, &list, &count);
	if (rc != SR_ERR_OK) {
		ERROR("Cannot find any asymmetric keys in configuration");
		return 0;
	}

	for (size_t i = 0; i < count; i++) {
		char *name = srx_get_str(session, "%s/name", list[i].xpath);
		char *public_key_format, *private_key_format;
		char *pub_key = NULL, *priv_key = NULL;
		sr_val_t *entry = &list[i];

		if (srx_isset(session, "%s/cleartext-private-key", entry->xpath) ||
		    srx_isset(session, "%s/public-key", entry->xpath))
			continue;

		public_key_format = srx_get_str(session, "%s/public-key-format", entry->xpath);
		if (!public_key_format)
			continue;

		private_key_format = srx_get_str(session, "%s/private-key-format", entry->xpath);
		if (!private_key_format) {
			free(public_key_format);
			continue;
		}

		if (strcmp(private_key_format, "infix-crypto-types:rsa-private-key-format") ||
		    strcmp(public_key_format, "infix-crypto-types:ssh-public-key-format"))
			continue;

		NOTE("SSH key (%s) does not exist, generating...", name);
		if (systemf("/usr/libexec/infix/mkkeys %s %s", SSH_PRIVATE_KEY, SSH_PUBLIC_KEY)) {
			ERROR("Failed generating SSH keys for %s", name);
			goto next;
		}

		priv_key = filerd(SSH_PRIVATE_KEY, filesz(SSH_PRIVATE_KEY));
		if (!priv_key)
			goto next;

		pub_key = filerd(SSH_PUBLIC_KEY, filesz(SSH_PUBLIC_KEY));
		if (!pub_key)
			goto next;

		rc = srx_set_str(session, priv_key, 0, "%s/cleartext-private-key", entry->xpath);
		if (rc) {
			ERROR("Failed setting private key for %s... rc: %d", name, rc);
			goto next;
		}

		rc = srx_set_str(session, pub_key, 0, "%s/public-key", entry->xpath);
		if (rc != SR_ERR_OK) {
			ERROR("Failed setting public key for %s... rc: %d", name, rc);
			goto next;
		}
	next:
		if (erase(SSH_PRIVATE_KEY))
			ERRNO("Failed removing SSH server private key");
		if (erase(SSH_PUBLIC_KEY))
			ERRNO("Failed removing SSH server public key");

		if (priv_key)
			free(priv_key);

		if (pub_key)
			free(pub_key);

		free(name);
		free(public_key_format);
		free(private_key_format);
	}

	if (list)
		sr_free_values(list, count);

	/* Second pass: generate X.509 certificates for TLS */
	list = NULL;
	count = 0;
	rc = sr_get_items(session, xpath, 0, 0, &list, &count);
	if (rc != SR_ERR_OK)
		return 0;

	for (size_t i = 0; i < count; i++) {
		char *name = srx_get_str(session, "%s/name", list[i].xpath);
		char *public_key_format = NULL, *private_key_format = NULL;
		char *pub_key = NULL, *priv_key = NULL, *cert = NULL;
		sr_val_t *entry = &list[i];

		if (srx_isset(session, "%s/cleartext-private-key", entry->xpath) ||
		    srx_isset(session, "%s/public-key", entry->xpath))
			goto next_x509;

		public_key_format = srx_get_str(session, "%s/public-key-format", entry->xpath);
		if (!public_key_format)
			goto next_x509;

		private_key_format = srx_get_str(session, "%s/private-key-format", entry->xpath);
		if (!private_key_format)
			goto next_x509;

		if (strcmp(private_key_format, "infix-crypto-types:rsa-private-key-format") ||
		    strcmp(public_key_format, "infix-crypto-types:x509-public-key-format"))
			goto next_x509;

		NOTE("X.509 certificate (%s) does not exist, generating...", name);
		if (systemf("/usr/libexec/infix/mkcert")) {
			ERROR("Failed generating X.509 certificate for %s", name);
			goto next_x509;
		}

		priv_key = filerd(TLS_PRIVATE_KEY, filesz(TLS_PRIVATE_KEY));
		if (!priv_key)
			goto next_x509;

		pub_key = filerd(TLS_CERTIFICATE, filesz(TLS_CERTIFICATE));
		if (!pub_key)
			goto next_x509;

		/* Use cert data also for public-key (X.509 SubjectPublicKeyInfo) */
		rc = srx_set_str(session, priv_key, 0, "%s/cleartext-private-key", entry->xpath);
		if (rc) {
			ERROR("Failed setting private key for %s... rc: %d", name, rc);
			goto next_x509;
		}

		rc = srx_set_str(session, pub_key, 0, "%s/public-key", entry->xpath);
		if (rc != SR_ERR_OK) {
			ERROR("Failed setting public key for %s... rc: %d", name, rc);
			goto next_x509;
		}

		cert = filerd(TLS_CERTIFICATE, filesz(TLS_CERTIFICATE));
		if (cert) {
			rc = srx_set_str(session, cert, 0,
					 "%s/certificates/certificate[name='self-signed']/cert-data",
					 entry->xpath);
			if (rc)
				ERROR("Failed setting cert-data for %s... rc: %d", name, rc);
		}

	next_x509:
		rmrf(TLS_TMPDIR);
		free(public_key_format);
		free(private_key_format);
		free(priv_key);
		free(pub_key);
		free(cert);
		free(name);
	}

	if (list)
		sr_free_values(list, count);

	return 0;
}

int keystore_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
		    sr_event_t event, struct confd *confd)
{
	struct lyd_node *changes, *change;
	int rc = SR_ERR_OK;

	if (diff && !lydx_find_xpathf(diff, XPATH_KEYSTORE_ASYM)
	         && !lydx_find_xpathf(diff, XPATH_KEYSTORE_SYM))
		return SR_ERR_OK;

	switch (event) {
	case SR_EV_UPDATE:
		return keystore_update(session, config, diff);
	case SR_EV_CHANGE:
		if (diff && lydx_find_xpathf(diff, XPATH_KEYSTORE_SYM))
			rc = interfaces_validate_keys(session, config);
		break;
	case SR_EV_ENABLED:
		break;
	case SR_EV_ABORT:
		rmrf(SSH_HOSTKEYS_NEXT);
		return SR_ERR_OK;
	case SR_EV_DONE:
		if (fexist(SSH_HOSTKEYS_NEXT)) {
			if (rmrf(SSH_HOSTKEYS))
				ERRNO("Failed to remove old SSH hostkeys: %d", errno);
			if (rename(SSH_HOSTKEYS_NEXT, SSH_HOSTKEYS))
				ERRNO("Failed switching to new %s", SSH_HOSTKEYS);
		}
		if (diff && lydx_find_xpathf(diff, XPATH_KEYSTORE_SYM))
			interfaces_validate_keys(NULL, config);
		return SR_ERR_OK;
	default:
		return SR_ERR_OK;
	}

	changes = lydx_get_descendant(config, "keystore", "asymmetric-keys", "asymmetric-key", NULL);
	LYX_LIST_FOR_EACH(changes, change, "asymmetric-key") {
		const char *name = lydx_get_cattr(change, "name");
		const char *pubfmt;

		pubfmt = lydx_get_cattr(change, "public-key-format");
		if (!strcmp(pubfmt, "infix-crypto-types:ssh-public-key-format"))
			gen_hostkey(name, change);
		else if (!strcmp(pubfmt, "infix-crypto-types:x509-public-key-format"))
			gen_webcert(name, change);
	}

	return rc;
}
