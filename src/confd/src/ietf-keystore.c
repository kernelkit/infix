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

static int change_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module_name,
		     const char *xpath, sr_event_t event, uint32_t request_id, void *_)
{
	int rc = SR_ERR_INTERNAL;
        sr_val_t *list = NULL;
        size_t count = 0;

	switch (event) {
	case SR_EV_UPDATE:
		/* Check SSH (and NETCONF) default hostkey pair */
		break;
	default:
		return SR_ERR_OK;
	}

	rc = sr_get_items(session, xpath, 0, 0, &list, &count);
        if (rc != SR_ERR_OK) {
                ERROR("Cannot find any asymmetric keys in configuration");
                return 0;
        }


        for (size_t i = 0; i < count; ++i) {
		sr_val_t *entry = &list[i];

		if (!srx_isset(session, "%s/cleartext-private-key", entry->xpath) && !srx_isset(session, "%s/public-key", entry->xpath)) {
			char *private_key_format, *public_key_format;

			public_key_format = srx_get_str(session, "%s/public-key-format", entry->xpath);
			if (!public_key_format)
				continue;
			private_key_format = srx_get_str(session, "%s/private-key-format", entry->xpath);
			if (!private_key_format) {
				free(public_key_format);
				continue;
			}

			if (!strcmp(private_key_format, "ietf-crypto-types:rsa-private-key-format") &&
			    !strcmp(public_key_format, "ietf-crypto-types:ssh-public-key-format")) {
				char *pub_key = NULL, *priv_key = NULL, *name;

				name = srx_get_str(session, "%s/name", entry->xpath);
				NOTE("SSH key (%s) does not exist, generating...", name);
				if (systemf("/usr/libexec/infix/mkkeys %s %s", SSH_PRIVATE_KEY, SSH_PUBLIC_KEY)) {
					ERROR("Failed to generate SSH keys for %s", name);
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
			}
			free(public_key_format);
			free(private_key_format);
		}
	}
	if (list)
		sr_free_values(list, count);


	return SR_ERR_OK;
}

int ietf_keystore_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->session, "ietf-keystore", "/ietf-keystore:keystore//.",
			SR_SUBSCR_UPDATE, change_cb, confd, &confd->sub);
fail:
	if(rc)
		ERROR("%s failed: %s", __func__, sr_strerror(rc));
	return rc;
}
