/* SPDX-License-Identifier: BSD-3-Clause */

#include <sys/stat.h>
#include <srx/common.h>
#include <srx/lyx.h>

#include <srx/srx_val.h>

#include "base64.h"
#include "core.h"

#define XPATH_KEYSTORE_  "/ietf-keystore:keystore/asymmetric-keys"
#define SSH_PRIVATE_KEY  "/tmp/ssh.key"
#define SSH_PUBLIC_KEY   "/tmp/ssh.pub"

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

	if (mkdir(SSH_HOSTKEYS_NEXT, 0600) && (errno != EEXIST)) {
		ERRNO("Failed creating %s", SSH_HOSTKEYS_NEXT);
		rc = SR_ERR_INTERNAL;
	}

	if (systemf("/usr/libexec/infix/mksshkey %s %s %s %s", name, SSH_HOSTKEYS_NEXT, public_key, private_key))
		rc = SR_ERR_INTERNAL;

	return rc;
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

	return 0;
}

int keystore_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
			 sr_event_t event, struct confd *confd)
{
	struct lyd_node *changes, *change;
	int rc = SR_ERR_OK;

	if (diff && !lydx_find_xpathf(diff, XPATH_KEYSTORE_))
		return SR_ERR_OK;

	switch (event) {
	case SR_EV_UPDATE:
		rc = keystore_update(session, config, diff);
		break;
	case SR_EV_CHANGE:
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
		return SR_ERR_OK;
	default:
		return SR_ERR_OK;
	}

	changes = lydx_get_descendant(config, "keystore", "asymmetric-keys", "asymmetric-key", NULL);
	LYX_LIST_FOR_EACH(changes, change, "asymmetric-key") {
		const char *name = lydx_get_cattr(change, "name");
		const char *type;

		type = lydx_get_cattr(change, "private-key-format");
		if (strcmp(type, "infix-crypto-types:rsa-private-key-format")) {
			INFO("Private key %s is not of SSH type (%s)", name, type);
			continue;
		}

		type = lydx_get_cattr(change, "public-key-format");
		if (strcmp(type, "infix-crypto-types:ssh-public-key-format")) {
			INFO("Public key %s is not of SSH type (%s)", name, type);
			continue;
		}

		gen_hostkey(name, change);
	}

	return rc;
}
