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

int infix_meta_change_cb(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	if (event == SR_EV_UPDATE)
		return set_version(session);

	return SR_ERR_OK;
}
