/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"

static struct confd confd;

static uint32_t hook_prio = CB_PRIO_PASSIVE;
static int num_changes;
static int cur_change;

static int startup_save_hook(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	if (systemf("sysrepocfg -X/cfg/startup-config.cfg -d startup -f json"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

uint32_t core_hook_prio(void)
{
	return hook_prio--;
}

/*
 * Run on UPDATE to see how many modules have changes in the inbound changeset
 * Run on DONE, after the last module callback has run, to activate changes.
 * For details, see https://github.com/sysrepo/sysrepo/issues/2188
 */
int core_commit_done(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	switch (event) {
	case SR_EV_UPDATE:
		num_changes++;
		return SR_ERR_OK;
	case SR_EV_DONE:
		cur_change++;
		if (cur_change == num_changes)
			break;
		return SR_ERR_OK;
	default:
		ERROR("core_commit_done() should not be called with event %d", event);
		return SR_ERR_SYS;
	}

	/* reset for next changeset */
	num_changes = cur_change = 0;

	/* skip reload in bootstrap, implicit reload in runlevel change */
	if (systemf("runlevel >/dev/null 2>&1"))
		return SR_ERR_OK;

	if (systemf("initctl -nbq reload"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	sr_session_ctx_t *startup;
	int rc = SR_ERR_SYS;

	openlog("confd", LOG_USER, 0);

	/* Save context with default running config datastore for all our models */
	*priv = (void *)&confd;
	confd.session = session;
	confd.conn    = sr_session_get_connection(session);
	confd.sub     = NULL;

	if (!confd.conn)
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

	/* YOUR_INIT GOES HERE */

	/* Set up hook to save startup-config to persistent backend store */
	rc = sr_session_start(confd.conn, SR_DS_STARTUP, &startup);
	if (rc)
		goto err;

	rc = sr_module_change_subscribe(startup, "ietf-system", "/ietf-system:system//.",
		startup_save_hook, NULL, CB_PRIO_PASSIVE, SR_SUBSCR_PASSIVE | SR_SUBSCR_DONE_ONLY, &confd.sub);
	if (rc) {
		ERROR("failed setting up startup-config hook: %s", sr_strerror(rc));
		goto err;
	}

	rc = sr_install_module(confd.conn, YANG_PATH_"/kernelkit-infix-deviations.yang", NULL, NULL);
	if (rc)
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
