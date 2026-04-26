/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <dirent.h>
#include <pwd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <regex.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#define MAX_MODEMS  	8
#define RUN_DIR		"/run/modemd"
#define SOCK		RUN_DIR "/modemd.sock"

#define MODULE		"infix-modem"
#define ROOT_XPATH	"/infix-modem:"
#define CFG_XPATH	ROOT_XPATH "modems"


static int xpath_get_index(const char *xpath)
{
	regmatch_t pmatch[2];
	regex_t regex;
	char buf[32];
	int ret, len, i;
	int index = -1;

	if (regcomp(&regex, "index='([0-9]+)'", REG_EXTENDED))
		return -1;

	ret = regexec(&regex, xpath, 2, pmatch, 0);
	if (!ret) {
		len = (pmatch[1].rm_eo - pmatch[1].rm_so);
		if (len < (int)(sizeof(buf)-1)) {
			for (i = 0; i < len; i++)
				buf[i] = xpath[pmatch[1].rm_so + i];

			buf[i] = '\0';
			index = (int) strtoul(buf, NULL, 10);
		}
	}
	regfree(&regex);

	return index;
}

static int node_index(struct lyd_node *node)
{
	const char *s;

	s = lydx_get_cattr(node, "index");
	if (!s || !s[0])
		return -1;

	return (int) strtoul(s, NULL, 10);
}

static int disable(void)
{
	NOTE("Disabling modemd");

	/* disable modem-manager */
	systemf("initctl -bfqn stop modem-manager");
	systemf("initctl -bfqn disable modem-manager");

	/* disable modemd */
	systemf("initctl -bnq stop modemd");
	systemf("initctl -bnq disable modemd");

	return SR_ERR_OK;
}

static int enable(void)
{
	int enabled, reload = 0;

	NOTE("Enabling modemd");

	/* enable modem-manager */
	enabled = !systemf("initctl -bfq status modem-manager");
	if (!enabled) {
		systemf("initctl -bfqn enable modem-manager");
		reload = 1;
	}
	/* enable modemd */
	enabled = !systemf("initctl -bfq status modemd");
	if (!enabled) {
		systemf("initctl -bfqn enable modemd");
		reload = 1;
	}
	/* reload if required */
	if (reload)
	    systemf("initctl -b reload");

	/* restart modem-manager */
	systemf("initctl -bfqn restart modem-manager");

	/* restart modemd */
	systemf("initctl -bfqn restart modemd");

	return SR_ERR_OK;
}

static int genconf(sr_data_t *cfg, struct lyd_node *diff)
{
	struct lyd_node *node, *tree;
	uint8_t enabled[MAX_MODEMS];
	int index;

	memset(enabled, 0, sizeof(enabled));

	tree = lydx_get_descendant(cfg->tree, "modems", "modem", NULL);
	LYX_LIST_FOR_EACH(tree, node, "modem") {
		index = node_index(node);
		if (index < MAX_MODEMS)
			enabled[index] = lydx_get_bool(node, "enabled") ? 1 : 0;
	}

	tree = lydx_get_descendant(diff, "modems", "modem", NULL);
	LYX_LIST_FOR_EACH(tree, node, "modem") {
		index = node_index(node);
		if (index < MAX_MODEMS && lydx_get_op(node) == LYDX_OP_DELETE)
			enabled[index] = 0;
	}

	for (index = 0; index < MAX_MODEMS; index++) {
		if (enabled[index]) {
			if (enable() == SR_ERR_OK) {
				return SR_ERR_OK;
			} else {
				ERROR("Cannot enable modem%d", index);
				break;
			}
		}
	}

	return disable();
}

static int infix_modem_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			      const char *xpath, sr_event_t event, unsigned request_id, void *confd)
{
	sr_data_t *cfg = NULL;
	struct lyd_node *diff = NULL;
	sr_error_t err;

	if (event != SR_EV_DONE) {
		return SR_ERR_OK;
	}
	err = sr_get_data(session, CFG_XPATH "//.", 0, 0, 0, &cfg);
	if (err) {
		ERROR("Can't get data");
		goto out;
	}
	err = srx_get_diff(session, &diff);
	if (err) {
		ERROR("Can't get diff");
		goto out;
	}
	err = genconf(cfg, diff);
	if (err) {
		ERROR("Can't gen conf");
		goto out;
	}
out:
	if (diff) lyd_free_tree(diff);
	if (cfg) sr_release_data(cfg);

	return err;
}

static int infix_modem_rpcsend(char *msg, int len)
{
	struct sockaddr_un addr;
	struct timeval tv;
	fd_set wfds;
	int sock, ret = -1;

	sock = socket(AF_UNIX, SOCK_STREAM, 0);
	if (sock < 0)
		return -1;

	addr.sun_family = AF_UNIX;
	strcpy(addr.sun_path, SOCK);

	if (connect(sock, &addr, sizeof(addr)) == 0) {
		tv.tv_sec = 5;
		tv.tv_usec = 0;
		FD_ZERO(&wfds);
		FD_SET(sock, &wfds);

		if (select(sock + 1, NULL, &wfds, NULL, &tv) > 0) {
			if (write(sock, msg, len) == len) {
				ret = 0;
			}
		}
	}
	close(sock);

	return ret;
}

static int infix_modem_rpc(const char *xpath, const char *rpc, const char *data)
{
	char msg[1024];
	int len;

	NOTE("Sending rpc %s to modemd", rpc);

	len = snprintf(msg, sizeof(msg),
		       "{ \"rpc\" : \"%s\", \"data\" : %s }",
		       rpc, data ? data : "null");

	if (infix_modem_rpcsend(msg, len) < 0) {
		ERROR("Unable to send rpc");
		return SR_ERR_INTERNAL;
	}

	return SR_ERR_OK;
}

static int infix_modem_sendsms(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
			       const sr_val_t *input, const size_t input_cnt, sr_event_t event,
			       unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	char data[1024];

	if (input_cnt < 3) {
		ERROR("Not enough input parameters");
		return SR_ERR_SYS;
	}

	snprintf(data, sizeof(data)-1,
		 "{ \"index\" : %s, \"number\" : \"%s\", \"text\" : \"%s\" }",
		 input[0].data.string_val,
		 input[1].data.string_val,
		 input[2].data.string_val);

	return infix_modem_rpc(xpath, "send-sms", data);
}

static int infix_modem_restart(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
			       const sr_val_t *input, const size_t input_cnt, sr_event_t event,
			       unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	char data[1024];

	if (input_cnt < 1) {
		ERROR("Not enough input parameters");
		return SR_ERR_SYS;
	}

	snprintf(data, sizeof(data)-1,
		 "{ \"index\" : %s }", input[0].data.string_val);

	return infix_modem_rpc(xpath, "restart", data);
}

static int infix_modem_reset(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
			     const sr_val_t *input, const size_t input_cnt, sr_event_t event,
			     unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	char data[1024];

	if (input_cnt < 1) {
		ERROR("Not enough input parameters");
		return SR_ERR_SYS;
	}

	snprintf(data, sizeof(data)-1,
		 "{ \"index\" : %s }", input[0].data.string_val);

	return infix_modem_rpc(xpath, "reset", data);
}

static void infix_modem_notif (sr_session_ctx_t *session, uint32_t sub_id,
			       const sr_ev_notif_type_t notif_type, const char *xpath,
			       const sr_val_t *values, const size_t values_cnt,
			       struct timespec *timestamp, void *confd)
{
	int index;

	index = xpath_get_index(xpath);
	if (index < 0) {
		ERROR("No index");
		return;
	}
	if (values_cnt < 1) {
		ERROR("No values");
		return;
	}

	NOTE("Notification from modem%d: %s",
	     index, values[0].data.string_val);
}

int modem_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->session, MODULE, CFG_XPATH, 0, infix_modem_change, confd, &confd->sub);
	REGISTER_NOTIF(confd->session, MODULE, CFG_XPATH "/modem/status-update", infix_modem_notif, confd, &confd->sub);
	REGISTER_RPC(confd->session, ROOT_XPATH "restart", infix_modem_restart, NULL, &confd->sub);
	REGISTER_RPC(confd->session, ROOT_XPATH "reset", infix_modem_reset, NULL, &confd->sub);
	REGISTER_RPC(confd->session, ROOT_XPATH "send-sms", infix_modem_sendsms, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}

int modem_gen(struct lyd_node *dif, struct lyd_node *cif, struct dagger *net)
{
        return 0;
}

int modem_gen_del(struct lyd_node *dif,  struct dagger *net)
{
        return 0;
}
