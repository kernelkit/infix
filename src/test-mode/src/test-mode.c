/*
* RPC control for test mode. This was first intended to be inside confd,
* but it was tricky to apply new  config in RPC (due to the behaviour of
* sr_replace_config), it called all callbacks for the new config and got
* stuck there until timeout. Maybe this change in the future, and this
* can live in confd.
*/

#include <libyang/tree_data.h>
#include <sysrepo.h>
#include <syslog.h>
#include <libite/lite.h>
#include <sysrepo_types.h>
#include <limits.h>

#include <srx/common.h>
#define SYSREPO_TIMEOUT 60000 /* 60s, this is the timeout we use in our frontends. */

sr_subscription_ctx_t *sub = NULL;
sr_conn_ctx_t *conn;

#define TEST_CONFIG_PATH "/etc/test-config.cfg"

static int test_reset(sr_session_ctx_t *session, uint32_t sub_id, const char *path,
		    const sr_val_t *input, const size_t input_cnt,
		    sr_event_t event, unsigned request_id,
		    sr_val_t **output, size_t *output_cnt,
		    void *priv)
{
	int rc = SR_ERR_OK;
	struct lyd_node *tree = NULL;
	const struct ly_ctx *ctx;
	sr_conn_ctx_t *conn = sr_session_get_connection(session);

	if (!conn)
		return SR_ERR_INTERNAL;

	ctx = sr_acquire_context(conn);
	if (!ctx)
		return SR_ERR_INTERNAL;

	rc = lyd_parse_data_path(ctx, TEST_CONFIG_PATH, LYD_JSON, LYD_PARSE_STRICT, LYD_VALIDATE_NO_STATE, &tree);
	if (rc != LY_SUCCESS)
	{
		ERROR("Failed to parse new configuration data");
		goto release_ctx;
	}
	rc = sr_session_switch_ds(session, SR_DS_RUNNING);
	if(rc)
	{
		ERROR("Failed to switch datastore");
		goto release_ctx;
	}
	rc = sr_replace_config(session, NULL, tree, SYSREPO_TIMEOUT);
	if (rc) {
		ERROR("Failed to replace configuration: %s", sr_strerror(rc));
		goto release_ctx;
	}
release_ctx:
	sr_release_context(conn);
	return rc;
}

static int test_override(sr_session_ctx_t *session, uint32_t sub_id, const char *path,
		      const sr_val_t *input, const size_t input_cnt,
		      sr_event_t event, unsigned request_id,
		      sr_val_t **output, size_t *output_cnt,
		      void *priv)
{
	touch("/mnt/aux/test-override-startup");

	return 0;
}

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	int rc = SR_ERR_SYS;

	if (!fexist("/mnt/aux/test-mode"))
		return SR_ERR_OK;
        rc = sr_rpc_subscribe(session, "/infix-test:test/reset", test_reset, NULL, 0, SR_SUBSCR_DEFAULT, &sub);
        if (rc) {
                ERROR("Failed subscribe for test-reset");
                goto out;
        }
        rc = sr_rpc_subscribe(session, "/infix-test:test/override-startup", test_override, NULL, 0, SR_SUBSCR_DEFAULT, &sub);
        if (rc) {
                ERROR("Failed subscribe for test-override");
                goto out;
        }

out:
	return rc;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
	if (sub)
		sr_unsubscribe(sub);
}
