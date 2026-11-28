/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * A GET is answered by asking yangerd, which gives sysrepo nothing to
 * compare, so a client with an on-change subscription hears nothing.
 * For the modules in push.h, also write the data yangerd sends when it has
 * changed into the operational datastore and let sysrepo tell the
 * subscribers what differs.
 *
 * A daemon of its own.  The write needs the lock of a module that a
 * client holds while it waits for statd to answer its GET.
 */

#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <syslog.h>
#include <unistd.h>

#include <ev.h>
#include <sysrepo.h>
#include <libite/lite.h>
#include <srx/common.h>

#include "push.h"
#include "yangerd.h"

#define PUSH_RETRY      1.0	/* s */
#define PUSH_TIMEOUT    5000	/* ms */

static const char *push_keys[] = PUSH_KEYS;

struct push {
	sr_conn_ctx_t    *conn;
	sr_session_ctx_t *ses;
	ev_io             io;
	ev_timer          timer;	/* opens a new watch */
};

/* Returns -1 when the write is worth another try */
static int push(sr_conn_ctx_t *conn, sr_session_ctx_t *ses, const char *key, const char *json)
{
	struct lyd_node *tree = NULL;
	const struct ly_ctx *ly;
	static unsigned int pushed;	/* keys with data in operational */
	unsigned int bit = 0;
	char xpath[128];
	int has_data;
	size_t i;
	int err;

	for (i = 0; i < NELEMS(push_keys); i++) {
		if (!strcmp(key, push_keys[i]))
			bit = 1u << i;
	}
	if (!bit)
		return 0;

	ly = sr_acquire_context(conn);
	if (!ly)
		return -1;

	if (yangerd_parse(ly, key, json, &tree) || (!tree && !(pushed & bit))) {
		sr_release_context(conn);
		return 0;
	}

	has_data = tree != NULL;
	if (has_data) {
		err = sr_edit_batch(ses, tree, "replace");
		lyd_free_all(tree);
	} else {
		snprintf(xpath, sizeof(xpath), "/%s", key);
		err = sr_delete_item(ses, xpath, 0);
	}
	sr_release_context(conn);

	err = err ?: sr_apply_changes(ses, PUSH_TIMEOUT);
	if (err) {
		sr_discard_changes(ses);
		if (err == SR_ERR_TIME_OUT || err == SR_ERR_LOCKED)
			return -1;
		ERROR("push: %s: %s", key, sr_strerror(err));
	} else if (has_data) {
		pushed |= bit;
	} else {
		pushed &= ~bit;
	}

	return 0;
}

static void watch_stop(struct ev_loop *loop, struct push *ctx)
{
	ev_io_stop(loop, &ctx->io);
	close(ctx->io.fd);
	ev_timer_set(&ctx->timer, PUSH_RETRY, 0.0);
	ev_timer_start(loop, &ctx->timer);
}

static void watch_cb(struct ev_loop *loop, ev_io *w, int revents)
{
	struct push *ctx = w->data;
	char *key, *json;
	int rc;

	if (yangerd_watch_read(w->fd, &key, &json)) {
		watch_stop(loop, ctx);
		return;
	}

	rc = push(ctx->conn, ctx->ses, key, json);
	free(key);
	free(json);
	if (rc)
		watch_stop(loop, ctx);
}

/* yangerd sends every key again on a new watch */
static void timer_cb(struct ev_loop *loop, ev_timer *w, int revents)
{
	struct push *ctx = w->data;
	int fd;

	fd = yangerd_watch(push_keys, NELEMS(push_keys));
	if (fd < 0) {
		ev_timer_set(&ctx->timer, PUSH_RETRY, 0.0);
		ev_timer_start(loop, &ctx->timer);
		return;
	}

	ev_io_init(&ctx->io, watch_cb, fd, EV_READ);
	ctx->io.data = ctx;
	ev_io_start(loop, &ctx->io);
}

static void signal_cb(struct ev_loop *loop, ev_signal *w, int revents)
{
	ev_break(loop, EVBREAK_ALL);
}

int main(void)
{
	struct ev_loop *loop = EV_DEFAULT;
	ev_signal sigterm, sigint;
	struct push ctx = { 0 };

	if (getenv("DEBUG"))
		debug = 1;
	openlog("statd-push", LOG_PID | LOG_NDELAY | (debug ? LOG_PERROR : 0), LOG_DAEMON);

	if (sr_connect(SR_CONN_DEFAULT, &ctx.conn)) {
		ERROR("push: no sysrepo connection");
		return EXIT_FAILURE;
	}
	if (sr_session_start(ctx.conn, SR_DS_OPERATIONAL, &ctx.ses)) {
		ERROR("push: no sysrepo session");
		sr_disconnect(ctx.conn);
		return EXIT_FAILURE;
	}

	ev_signal_init(&sigterm, signal_cb, SIGTERM);
	ev_signal_start(loop, &sigterm);
	ev_signal_init(&sigint, signal_cb, SIGINT);
	ev_signal_start(loop, &sigint);

	ev_timer_init(&ctx.timer, timer_cb, 0.0, 0.0);
	ctx.timer.data = &ctx;
	ev_timer_start(loop, &ctx.timer);

	pidfile(NULL);
	ev_run(loop, 0);

	sr_session_stop(ctx.ses);
	sr_disconnect(ctx.conn);

	return EXIT_SUCCESS;
}
