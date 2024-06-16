/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"

#define META_XPATH  "/infix-meta:meta/version"


static int set_version(sr_session_ctx_t *session)
{
	int rc;

	rc = sr_set_item_str(session, META_XPATH, VERSION, NULL, 0);
	if (rc) {
		ERROR("Failed setting .cfg version %s: %d", VERSION, rc);
		return rc;
	}

	return SR_ERR_OK;
}

static int change_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		       const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	if (event == SR_EV_UPDATE)
		return set_version(session);

	return SR_ERR_OK;
}

int infix_meta_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->session, "infix-meta", META_XPATH, SR_SUBSCR_UPDATE,
			change_cb, confd, &confd->sub);
	return SR_ERR_OK;
fail:
	ERROR("%s(): failed. %s", __func__, sr_strerror(rc));
	return rc;
}
