/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include "../lib/common.h"
#include "../lib/lyx.h"

/* TODO: This will be removed, it's just a "proof of concept" */
#define TMP_ETH0_XPATH "/ietf-interfaces:interfaces/interface[name='eth0']"
#define TMP_ETH0_OPERSTATE "/sys/class/net/eth0/operstate"

/* TODO: This will be removed and replaced by json parsing */
static int read_proc(const char *path, char *buf, size_t size)
{
	FILE *file;

	file = fopen(path, "r");
	if (!file)
		return -1;

	if (!fgets(buf, size, file)) {
		fclose(file);
		return -1;
	}
	buf[strcspn(buf, "\n")] = '\0';
	fclose(file);

	return 0;
}

static int stat_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *path, const char *request_path, uint32_t request_id,
		    struct lyd_node **parent, void *priv)
{
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *con;
	char buf[256] = {};
	int first = 1;
	int err;

	con = sr_session_get_connection(session);
	if (!con)
		return SR_ERR_INTERNAL;

	/* TODO: This will be replaced by "foreach interface" and json data */
	err = read_proc(TMP_ETH0_OPERSTATE, buf, sizeof(buf) - 1);
	if (err) {
		ERROR("error reading proc");
		return SR_ERR_SYS;
	}

	ctx = sr_acquire_context(con);
	if (!ctx)
		return SR_ERR_INTERNAL;

	err = lydx_new_path(ctx, parent, &first, TMP_ETH0_XPATH, "oper-status", buf);
	sr_release_context(con);
	if (err) {
		ERROR("failed building data tree, libyang error %d", err);
		return SR_ERR_LY;
	}

	return SR_ERR_OK;
}

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	sr_subscription_ctx_t *sub = NULL;
	sr_session_ctx_t *op;
	sr_conn_ctx_t *conn;
	int rc;

	openlog("statd", LOG_USER, 0);
	INFO("Sysrepo callback hello");

	conn = sr_session_get_connection(session);
	if (!conn) {
		ERROR("unable to connect sr session");
		return SR_ERR_INTERNAL;
	}

	rc = sr_session_start(conn, SR_DS_OPERATIONAL, &op);
	if (rc) {
		ERROR("unable to start session: %s", sr_strerror(rc));
		return rc;
	}

	rc = sr_oper_get_subscribe(session, "ietf-interfaces", TMP_ETH0_XPATH,
				   stat_cb, NULL, 0 | SR_SUBSCR_DEFAULT, &sub);
	if (rc) {
		ERROR("failed subscribing to %s oper: %s", TMP_ETH0_XPATH, sr_strerror(rc));
		return rc;
	}

	rc = srx_require_modules(conn, core_reqs);
	if (rc) {
		ERROR("required modules failed: %s", sr_strerror(rc));
		sr_unsubscribe(sub);
		return rc;
	}

	return SR_ERR_OK;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
        sr_unsubscribe((sr_subscription_ctx_t *)priv);
	closelog();
}
