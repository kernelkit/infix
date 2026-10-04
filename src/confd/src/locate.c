/* SPDX-License-Identifier: BSD-3-Clause */
#include <errno.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <unistd.h>

#include <jansson.h>
#include <srx/common.h>

#include "core.h"

#define IITOD_SOCKET	"/run/iitod.sock"
#define IITOD_TIMEOUT	2	/* seconds */

static int fail(sr_session_ctx_t *session, const char *msg)
{
	sr_session_set_netconf_error(session, "application", "operation-failed",
				     NULL, NULL, msg, 0);
	return SR_ERR_OPERATION_FAILED;
}

/*
 * One request, one reply, over the iitod control socket.  Returns the
 * parsed reply, or NULL with *err set.  iitod is disabled on systems
 * where kernel LED support is unreliable, then there is no socket.
 */
static json_t *iitod_call(json_t *req, const char **err)
{
	struct sockaddr_un sun = { .sun_family = AF_UNIX, .sun_path = IITOD_SOCKET };
	struct timeval tv = { .tv_sec = IITOD_TIMEOUT };
	char buf[4096], *msg;
	json_t *reply = NULL;
	size_t len = 0;
	ssize_t n;
	int sd;

	sd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
	if (sd < 0) {
		*err = "failed creating socket";
		return NULL;
	}

	/* Do not let a stuck iitod block confd, this also bounds connect() */
	setsockopt(sd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
	setsockopt(sd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv));

	if (connect(sd, (struct sockaddr *)&sun, sizeof(sun))) {
		if (errno == ENOENT)
			*err = "LED control is not available on this device";
		else if (errno == ECONNREFUSED)
			*err = "the LED daemon is not running";
		else
			*err = "failed connecting to the LED daemon";
		goto done;
	}

	msg = json_dumps(req, JSON_COMPACT);
	if (!msg || dprintf(sd, "%s\n", msg) < 0) {
		*err = "failed sending request to the LED daemon";
		free(msg);
		goto done;
	}
	free(msg);

	while (len < sizeof(buf) - 1) {
		n = read(sd, &buf[len], sizeof(buf) - 1 - len);
		if (n <= 0)
			break;
		len += n;
	}
	buf[len] = 0;

	reply = json_loads(buf, 0, NULL);
	if (!reply)
		*err = "no valid reply from the LED daemon";
done:
	close(sd);
	return reply;
}

static int rpc_locate(sr_session_ctx_t *session, uint32_t sub_id, const char *path,
		      const sr_val_t *input, const size_t input_cnt, sr_event_t event,
		      unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	const char *err, *user, *via;
	uint32_t duration = 0;
	json_t *req, *reply;
	bool enable = false;
	int rc = SR_ERR_OK;

	if (event != SR_EV_RPC)
		return SR_ERR_OK;

	/* sysrepo passes the YANG defaults for leaves not given */
	for (size_t i = 0; i < input_cnt; i++) {
		const char *leaf = strrchr(input[i].xpath, '/');

		if (!leaf)
			continue;
		if (!strcmp(leaf, "/enable"))
			enable = input[i].data.bool_val;
		else if (!strcmp(leaf, "/duration"))
			duration = input[i].data.uint32_val;
	}

	user = rpc_user(session, &via);
	AUDIT("Locate %s by user \"%s\" over %s.", enable ? "started" : "stopped",
	      user ?: "unknown", via);

	/* iitod only arms the timeout when enabling */
	req = json_pack("{s:s, s:{s:b, s:I}}", "method", "locate", "params",
			"enable", enable, "timeout", (json_int_t)duration);
	reply = iitod_call(req, &err);
	json_decref(req);
	if (!reply)
		return fail(session, err);

	if (json_object_get(reply, "error"))
		rc = fail(session, json_string_value(json_object_get(reply, "error")) ? : "failed");

	json_decref(reply);
	return rc;
}

int locate_rpc_init(struct confd *confd)
{
	return register_rpc(confd->session, "/infix-hardware:locate",
			    rpc_locate, NULL, &confd->sub);
}
