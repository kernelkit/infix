/* SPDX-License-Identifier: BSD-3-Clause */

#include <sys/types.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <regex.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#define RUN_DIR     "/run/modemd"
#define SOCK        RUN_DIR "/modemd.sock"

#define HW_BASE     "ietf-hardware"
#define HW_MODULE   "infix-hardware"
#define HW_COMP     "/ietf-hardware:hardware/component"
#define IH_MODEM    HW_COMP "/infix-hardware:modem"
#define IH_COMP_ACTION   HW_COMP "/infix-hardware:"


static int component_index(const char *xpath)
{
	regmatch_t pmatch[2];
	regex_t regex;
	char name[32];
	int index = -1;
	int len;

	if (regcomp(&regex, "name='([^']*)'", REG_EXTENDED))
		return -1;

	if (regexec(&regex, xpath, 2, pmatch, 0) == 0) {
		len = pmatch[1].rm_eo - pmatch[1].rm_so;
		if (len < (int)(sizeof(name) - 1)) {
			memcpy(name, xpath + pmatch[1].rm_so, len);
			name[len] = '\0';
			sscanf(name, "modem%d", &index);
		}
	}
	regfree(&regex);

	return index;
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
			if (write(sock, msg, len) == len)
				ret = 0;
		}
	}
	close(sock);

	return ret;
}

static int infix_modem_rpc(const char *rpc, const char *data)
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
	int index;

	index = component_index(xpath);
	if (index < 0) {
		ERROR("Cannot parse modem index from xpath: %s", xpath);
		return SR_ERR_INVAL_ARG;
	}
	if (input_cnt < 2) {
		ERROR("send-sms: not enough input parameters");
		return SR_ERR_SYS;
	}

	snprintf(data, sizeof(data) - 1,
		 "{ \"index\" : %d, \"number\" : \"%s\", \"text\" : \"%s\" }",
		 index,
		 input[0].data.string_val,
		 input[1].data.string_val);

	return infix_modem_rpc("send-sms", data);
}

static int infix_modem_restart(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
			       const sr_val_t *input, const size_t input_cnt, sr_event_t event,
			       unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	char data[64];
	int index;

	index = component_index(xpath);
	if (index < 0) {
		ERROR("Cannot parse modem index from xpath: %s", xpath);
		return SR_ERR_INVAL_ARG;
	}

	snprintf(data, sizeof(data), "{ \"index\" : %d }", index);
	return infix_modem_rpc("restart", data);
}

static int infix_modem_reset(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
			     const sr_val_t *input, const size_t input_cnt, sr_event_t event,
			     unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	char data[64];
	int index;

	index = component_index(xpath);
	if (index < 0) {
		ERROR("Cannot parse modem index from xpath: %s", xpath);
		return SR_ERR_INVAL_ARG;
	}

	snprintf(data, sizeof(data), "{ \"index\" : %d }", index);
	return infix_modem_rpc("reset", data);
}

static void infix_modem_notif(sr_session_ctx_t *session, uint32_t sub_id,
			      const sr_ev_notif_type_t notif_type, const char *xpath,
			      const sr_val_t *values, const size_t values_cnt,
			      struct timespec *timestamp, void *confd)
{
	int index;

	index = component_index(xpath);
	if (index < 0) {
		ERROR("Cannot parse modem index from xpath: %s", xpath);
		return;
	}
	if (values_cnt < 1) {
		ERROR("No values in status-update notification");
		return;
	}

	NOTE("Notification from modem%d: %s", index, values[0].data.string_val);
}

int modem_init(struct confd *confd)
{
	const struct lys_module *mod;
	sr_conn_ctx_t *conn;
	const struct ly_ctx *ly_ctx;
	int rc;

	conn = sr_session_get_connection(confd->session);
	ly_ctx = sr_acquire_context(conn);
	mod = ly_ctx_get_module_implemented(ly_ctx, HW_MODULE);
	sr_release_context(conn);

	if (!mod || lys_feature_value(mod, "modem") != LY_SUCCESS)
		return SR_ERR_OK;

	REGISTER_NOTIF(confd->session, HW_BASE,
		       IH_MODEM "/status-update",
		       infix_modem_notif, confd, &confd->sub);
	REGISTER_RPC(confd->session, IH_COMP_ACTION "restart",
		     infix_modem_restart, NULL, &confd->sub);
	REGISTER_RPC(confd->session, IH_COMP_ACTION "reset",
		     infix_modem_reset, NULL, &confd->sub);
	REGISTER_RPC(confd->session, IH_COMP_ACTION "send-sms",
		     infix_modem_sendsms, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
