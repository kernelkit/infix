/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"
#include "../lib/srx_module.h"
#include "../lib/common.h"

static struct confd confd;

uint32_t core_hook_prio(void)
{
	static uint32_t hook_prio = CB_PRIO_PASSIVE;

	return hook_prio--;
}

int core_startup_save(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	if (systemf("sysrepocfg -X/cfg/startup-config.cfg -d startup -f json"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

int core_pre_hook(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	return 0;
}

/*
 * Run on UPDATE to see how many modules have changes in the inbound changeset
 * Run on DONE, after the last module callback has run, to activate changes.
 * For details, see https://github.com/sysrepo/sysrepo/issues/2188
 */
int core_post_hook(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	static int num_changes = 0;

	switch (event) {
	case SR_EV_CHANGE:
		num_changes++;
		return SR_ERR_OK;
	case SR_EV_ABORT:
		num_changes = 0;
		return SR_ERR_OK;
	case SR_EV_DONE:
		if (--num_changes == 0)
			break;
		return SR_ERR_OK;
	default:
		ERROR("core_commit_done() should not be called with event %d", event);
		return SR_ERR_SYS;
	}

	/* skip reload in bootstrap, implicit reload in runlevel change */
	if (systemf("runlevel >/dev/null 2>&1"))
		return SR_ERR_OK;

	if (systemf("initctl -nbq reload"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	int rc = SR_ERR_SYS;

	openlog("confd", LOG_USER, 0);

	/* Save context with default running config datastore for all our models */
	*priv = (void *)&confd;
	confd.session = session;
	confd.conn    = sr_session_get_connection(session);
	confd.sub     = NULL;
	confd.fsub    = NULL;

	if (!confd.conn)
		goto err;

	/* The startup datastore is used for the core_startup_save() hook */
	rc = sr_session_start(confd.conn, SR_DS_STARTUP, &confd.startup);
	if (rc)
		goto err;

	/* Used by, e.g., ietf-interfaces, to update interfaces types */
	rc = sr_session_start(confd.conn, SR_DS_CANDIDATE, &confd.cand);
	if (rc)
		goto err;

	confd.aug = aug_init(NULL, "", 0);
	if (!confd.aug)
		goto err;

	rc = ietf_interfaces_init(&confd);
	if (rc)
		goto err;
	rc = ietf_system_init(&confd);
	if (rc)
		goto err;
	rc = infix_dhcp_init(&confd);
	if (rc)
		goto err;
	rc = infix_factory_init(&confd);
	if (rc)
		goto err;
	rc = infix_system_sw_init(&confd);
	if (rc)
		goto err;

	/* YOUR_INIT GOES HERE */

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(rc));
	sr_unsubscribe(confd.sub);
	sr_unsubscribe(confd.fsub);

	return rc;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
	struct confd *confd = (struct confd *)priv;

        sr_unsubscribe(confd->sub);
        sr_unsubscribe(confd->fsub);
	closelog();
}
