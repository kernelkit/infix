/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * Background GPS monitor for statd.
 *
 * Maintains a persistent connection to gpsd (localhost:2947) and caches
 * GPS device status to /run/gps-status.json.  The yanger ietf_hardware
 * module reads this cache instead of spawning gpspipe, avoiding blocking
 * the operational datastore.
 *
 * Activated on SIGHUP when sysrepo running config contains a hardware
 * component with class infix-hardware:gps, reconnects automatically
 * if gpsd restarts.
 */

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <errno.h>
#include <fcntl.h>
#include <sys/stat.h>
#include <sys/socket.h>
#include <netinet/in.h>

#include <ev.h>
#include <jansson.h>

#include <srx/common.h>

#include "gpsd.h"

static int gps_device_present(void)
{
	struct stat st;
	int i;

	for (i = 0; i < 4; i++) {
		char path[32];

		snprintf(path, sizeof(path), "/dev/gps%d", i);
		if (stat(path, &st) == 0)
			return 1;
	}

	return 0;
}

static void cache_write(struct gpsd_ctx *ctx)
{
	char tmp[] = GPSD_CACHE_FILE ".XXXXXX";
	int fd;

	if (!ctx->cache)
		return;

	fd = mkstemp(tmp);
	if (fd < 0) {
		ERROR("gpsd: failed to create temp file: %s", strerror(errno));
		return;
	}

	if (json_dumpfd(ctx->cache, fd, JSON_INDENT(2)) < 0) {
		ERROR("gpsd: failed to write cache");
		close(fd);
		unlink(tmp);
		return;
	}

	close(fd);
	if (rename(tmp, GPSD_CACHE_FILE) < 0) {
		ERROR("gpsd: failed to rename cache: %s", strerror(errno));
		unlink(tmp);
	}
}

static void handle_devices(struct gpsd_ctx *ctx, json_t *msg)
{
	json_t *devices = json_object_get(msg, "devices");
	size_t i;

	if (!json_is_array(devices))
		return;

	for (i = 0; i < json_array_size(devices); i++) {
		json_t *dev = json_array_get(devices, i);
		const char *path = json_string_value(json_object_get(dev, "path"));
		json_t *entry, *driver, *activated;

		if (!path)
			continue;

		entry = json_object_get(ctx->cache, path);
		if (!entry) {
			entry = json_object();
			json_object_set_new(ctx->cache, path, entry);
		}

		driver = json_object_get(dev, "driver");
		if (json_is_string(driver))
			json_object_set(entry, "driver", driver);

		/* activated is a timestamp string when active, absent when not */
		activated = json_object_get(dev, "activated");
		if (activated && json_is_string(activated) &&
		    strlen(json_string_value(activated)) > 0)
			json_object_set_new(entry, "activated", json_true());
		else
			json_object_set_new(entry, "activated", json_false());
	}
}

static void handle_tpv(struct gpsd_ctx *ctx, json_t *msg)
{
	const char *path = json_string_value(json_object_get(msg, "device"));
	json_t *entry, *val;

	if (!path)
		return;

	entry = json_object_get(ctx->cache, path);
	if (!entry) {
		entry = json_object();
		json_object_set_new(ctx->cache, path, entry);
	}

	/* Fix mode: 0=unknown, 1=none, 2=2D, 3=3D */
	val = json_object_get(msg, "mode");
	if (json_is_integer(val))
		json_object_set(entry, "mode", val);

	val = json_object_get(msg, "lat");
	if (json_is_number(val))
		json_object_set(entry, "lat", val);

	val = json_object_get(msg, "lon");
	if (json_is_number(val))
		json_object_set(entry, "lon", val);

	val = json_object_get(msg, "altHAE");
	if (json_is_number(val))
		json_object_set(entry, "altHAE", val);
}

static void handle_sky(struct gpsd_ctx *ctx, json_t *msg)
{
	const char *path = json_string_value(json_object_get(msg, "device"));
	json_t *entry, *sats;
	size_t i, visible, used;

	if (!path)
		return;

	entry = json_object_get(ctx->cache, path);
	if (!entry) {
		entry = json_object();
		json_object_set_new(ctx->cache, path, entry);
	}

	sats = json_object_get(msg, "satellites");
	if (!json_is_array(sats))
		return;

	visible = json_array_size(sats);
	used = 0;
	for (i = 0; i < visible; i++) {
		json_t *sat = json_array_get(sats, i);

		if (json_is_true(json_object_get(sat, "used")))
			used++;
	}

	json_object_set_new(entry, "satellites_visible", json_integer(visible));
	json_object_set_new(entry, "satellites_used", json_integer(used));
}

static void process_line(struct gpsd_ctx *ctx, const char *line)
{
	json_error_t err;
	json_t *msg;
	const char *cls;

	msg = json_loads(line, 0, &err);
	if (!msg)
		return;

	cls = json_string_value(json_object_get(msg, "class"));
	if (!cls)
		goto out;

	if (strcmp(cls, "DEVICES") == 0)
		handle_devices(ctx, msg);
	else if (strcmp(cls, "TPV") == 0)
		handle_tpv(ctx, msg);
	else if (strcmp(cls, "SKY") == 0)
		handle_sky(ctx, msg);

	cache_write(ctx);
out:
	json_decref(msg);
}

static void gpsd_disconnect(struct gpsd_ctx *ctx)
{
	if (!ctx->connected)
		return;

	ev_io_stop(ctx->loop, &ctx->sock_watcher);
	close(ctx->sock_fd);
	ctx->sock_fd = -1;
	ctx->connected = 0;
	ctx->buf_used = 0;
	DEBUG("gpsd: disconnected");
}

static void sock_read_cb(struct ev_loop *, ev_io *w, int)
{
	struct gpsd_ctx *ctx = (struct gpsd_ctx *)w->data;
	char *start, *nl;
	ssize_t n;

	n = read(ctx->sock_fd, ctx->buf + ctx->buf_used,
		 sizeof(ctx->buf) - ctx->buf_used - 1);
	if (n <= 0) {
		if (n < 0 && (errno == EAGAIN || errno == EINTR))
			return;
		DEBUG("gpsd: connection lost (%s)", n == 0 ? "EOF" : strerror(errno));
		gpsd_disconnect(ctx);
		return;
	}

	ctx->buf_used += n;
	ctx->buf[ctx->buf_used] = '\0';

	/* Process complete lines (gpsd sends one JSON object per line) */
	start = ctx->buf;
	while ((nl = strchr(start, '\n')) != NULL) {
		*nl = '\0';
		if (nl > start)
			process_line(ctx, start);
		start = nl + 1;
	}

	/* Shift remaining partial line to beginning of buffer */
	if (start != ctx->buf) {
		ctx->buf_used -= (start - ctx->buf);
		memmove(ctx->buf, start, ctx->buf_used);
	}

	/* Buffer overflow protection */
	if (ctx->buf_used >= sizeof(ctx->buf) - 1) {
		ERROR("gpsd: read buffer overflow, resetting");
		ctx->buf_used = 0;
	}
}

static int gpsd_connect(struct gpsd_ctx *ctx)
{
	static const char watch_cmd[] = "?WATCH={\"enable\":true,\"json\":true};\n";
	struct sockaddr_in addr;
	int fd, flags;

	fd = socket(AF_INET, SOCK_STREAM, 0);
	if (fd < 0) {
		DEBUG("gpsd: socket(): %s", strerror(errno));
		return -1;
	}

	memset(&addr, 0, sizeof(addr));
	addr.sin_family = AF_INET;
	addr.sin_port = htons(GPSD_PORT);
	addr.sin_addr.s_addr = htonl(INADDR_LOOPBACK);

	if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
		DEBUG("gpsd: connect(): %s", strerror(errno));
		close(fd);
		return -1;
	}

	/* Enable JSON watch mode (socket still blocking, localhost = instant) */
	if (write(fd, watch_cmd, strlen(watch_cmd)) < 0) {
		ERROR("gpsd: failed to send WATCH command: %s", strerror(errno));
		close(fd);
		return -1;
	}

	/* Switch to non-blocking for ev_io */
	flags = fcntl(fd, F_GETFL, 0);
	if (flags >= 0)
		fcntl(fd, F_SETFL, flags | O_NONBLOCK);

	/* Clear stale cache data from previous connection */
	json_object_clear(ctx->cache);

	ctx->sock_fd = fd;
	ctx->connected = 1;
	ctx->buf_used = 0;

	ev_io_init(&ctx->sock_watcher, sock_read_cb, fd, EV_READ);
	ctx->sock_watcher.data = ctx;
	ev_io_start(ctx->loop, &ctx->sock_watcher);

	INFO("gpsd: connected");
	return 0;
}

static void check_timer_cb(struct ev_loop *, ev_timer *w, int)
{
	struct gpsd_ctx *ctx = (struct gpsd_ctx *)w->data;

	if (ctx->connected)
		return;

	if (!gps_device_present()) {
		unlink(GPSD_CACHE_FILE);
		return;
	}

	gpsd_connect(ctx);
}

/*
 * Check sysrepo running config for hardware components with class
 * infix-hardware:gps.  Returns 1 if at least one is found.
 */
static int has_gps_component(sr_conn_ctx_t *conn)
{
	sr_session_ctx_t *ses;
	sr_val_t *vals = NULL;
	size_t cnt = 0;
	int found;

	if (sr_session_start(conn, SR_DS_RUNNING, &ses))
		return 0;

	sr_get_items(ses,
		     "/ietf-hardware:hardware/component[class='infix-hardware:gps']/name",
		     0, 0, &vals, &cnt);

	found = cnt > 0;
	sr_free_values(vals, cnt);
	sr_session_stop(ses);

	return found;
}

static void gpsd_activate(struct gpsd_ctx *ctx)
{
	if (ctx->active)
		return;

	ctx->active = 1;
	ev_timer_start(ctx->loop, &ctx->check_timer);

	if (gps_device_present())
		gpsd_connect(ctx);

	INFO("gpsd: GPS monitoring activated");
}

static void gpsd_deactivate(struct gpsd_ctx *ctx)
{
	if (!ctx->active)
		return;

	ctx->active = 0;
	gpsd_disconnect(ctx);
	ev_timer_stop(ctx->loop, &ctx->check_timer);
	json_object_clear(ctx->cache);
	unlink(GPSD_CACHE_FILE);

	INFO("gpsd: GPS monitoring deactivated");
}

void gpsd_reload(struct gpsd_ctx *ctx)
{
	if (has_gps_component(ctx->sr_conn))
		gpsd_activate(ctx);
	else
		gpsd_deactivate(ctx);
}

int gpsd_init(struct gpsd_ctx *ctx, struct ev_loop *loop, sr_conn_ctx_t *conn)
{
	memset(ctx, 0, sizeof(*ctx));
	ctx->loop = loop;
	ctx->sock_fd = -1;
	ctx->sr_conn = conn;

	ctx->cache = json_object();
	if (!ctx->cache) {
		ERROR("gpsd: failed to create cache object");
		return -1;
	}

	ev_timer_init(&ctx->check_timer, check_timer_cb,
		      GPSD_CHECK_INTERVAL, GPSD_CHECK_INTERVAL);
	ctx->check_timer.data = ctx;
	/* Timer not started here -- gpsd_reload() activates when needed */

	INFO("gpsd: GPS monitor initialized");
	return 0;
}

void gpsd_exit(struct gpsd_ctx *ctx)
{
	gpsd_deactivate(ctx);

	if (ctx->cache) {
		json_decref(ctx->cache);
		ctx->cache = NULL;
	}

	INFO("gpsd: GPS monitor stopped");
}
