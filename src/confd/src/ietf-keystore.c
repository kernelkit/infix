/* SPDX-License-Identifier: BSD-3-Clause */

#include <sys/stat.h>
#include <srx/common.h>
#include <srx/lyx.h>

#include <srx/srx_val.h>

#include "base64.h"
#include "core.h"

#define XPATH_KEYSTORE_  "/ietf-keystore:keystore/asymmetric-keys"

/* return file size */
static size_t filesz(const char *fn)
{
	struct stat st;

	if (stat(fn, &st))
		st.st_size = BUFSIZ;
	return st.st_size;
}

/* sanity check, must exist and be of non-zero size */
static bool fileok(const char *fn)
{
	if (!fexist(fn))
		return false;

	/* Minimum size of BEGIN + END markers */
	if (filesz(fn) < 60)
		return false;

	return true;
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
	const char *priv_keyfile = "/cfg/ssl/private/netconf.key";
	const char *pub_keyfile  = "/cfg/ssl/public/netconf.pub";
	char *pub_key = NULL, *priv_key = NULL;
	int rc = SR_ERR_INTERNAL;

	switch (event) {
	case SR_EV_UPDATE:
		/* Check NETCONF default hostkey pair */
		break;
	default:
		return SR_ERR_OK;
	}

	if (srx_isset(session, XPATH_KEYSTORE_"/asymmetric-key[name='genkey']/cleartext-private-key") &&
	    srx_isset(session, XPATH_KEYSTORE_"/asymmetric-key[name='genkey']/public-key")) {
		return SR_ERR_OK; /* already set */
	}
	WARN("NETCONF private and public host keys missing in confiugration.");

	if (!fileok(priv_keyfile) || !fileok(pub_keyfile)) {
		NOTE("Generating NETCONF SSH host keys ...");
		if (systemf("/libexec/infix/mkkeys %s %s", priv_keyfile, pub_keyfile))
			goto err;
	} else {
		NOTE("Using existing SSH host keys for NETCONF.");
	}

	priv_key = filerd(priv_keyfile, filesz(priv_keyfile));
	if (!priv_key)
		goto err;

	pub_key = filerd(pub_keyfile, filesz(pub_keyfile));
	if (!pub_key)
		goto err;

	xpath = XPATH_KEYSTORE_"/asymmetric-key[name='genkey']/cleartext-private-key";
	rc = sr_set_item_str(session, xpath, priv_key, NULL, SR_EDIT_NON_RECURSIVE);
	if (rc) {
		ERROR("Failed setting private key ... rc: %d", rc);
		goto err;
	}

	xpath = XPATH_KEYSTORE_"/asymmetric-key[name='genkey']/public-key";
	rc = sr_set_item_str(session, xpath, pub_key, NULL, SR_EDIT_NON_RECURSIVE);
	if (rc != SR_ERR_OK) {
		ERROR("Failed setting public key ... rc: %d", rc);
		goto err;
	}

err:
	if (pub_key)
		free(pub_key);
	if (priv_key)
		free(priv_key);

	if (rc != SR_ERR_OK)
		return rc;

	return SR_ERR_OK;
}

int ietf_keystore_init(struct confd *confd)
{
	int rc;

	rc = sr_module_change_subscribe(confd->session, "ietf-keystore", "/ietf-keystore:keystore//.",
					change_cb, confd, 0, SR_SUBSCR_UPDATE, &confd->sub);
	if (rc)
		ERROR("%s failed: %s", __func__, sr_strerror(rc));
	return rc;
}
