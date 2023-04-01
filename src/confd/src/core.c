/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"

static struct confd confd;

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	int rc = SR_ERR_SYS;

	openlog("confd", LOG_USER, 0);

	confd.session = session;
	confd.conn    = sr_session_get_connection(session);
	confd.sub     = NULL;

	if (!confd.conn)
		goto err;

	confd.aug = aug_init(NULL, "", 0);
	if (!confd.aug)
		goto err;

	*priv = (void *)&confd;

	if (ietf_system_init(&confd))
		goto err;

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(rc));
	sr_unsubscribe(confd.sub);

	return rc;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
        sr_unsubscribe((sr_subscription_ctx_t *)priv);
	closelog();
}
