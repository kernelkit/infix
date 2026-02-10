/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_GPSD_H_
#define STATD_GPSD_H_

#include <ev.h>
#include <jansson.h>
#include <sysrepo.h>

#define GPSD_CACHE_FILE    "/run/gps-status.json"
#define GPSD_PORT          2947
#define GPSD_CHECK_INTERVAL 10.0  /* Seconds between device/connection checks */
#define GPSD_READ_BUFSZ    4096

struct gpsd_ctx {
	struct ev_loop *loop;
	ev_timer        check_timer;  /* Periodic check for GPS devices / reconnect */
	ev_io           sock_watcher; /* Read watcher on gpsd socket */
	int             sock_fd;      /* TCP socket to gpsd */
	char            buf[GPSD_READ_BUFSZ]; /* Line accumulation buffer */
	size_t          buf_used;
	json_t         *cache;        /* Accumulated GPS data, keyed by device path */
	int             connected;
	sr_conn_ctx_t  *sr_conn;      /* Sysrepo connection for config queries */
	int             active;       /* GPS monitoring enabled (config has gps component) */
};

int  gpsd_init(struct gpsd_ctx *ctx, struct ev_loop *loop, sr_conn_ctx_t *conn);
void gpsd_reload(struct gpsd_ctx *ctx);
void gpsd_exit(struct gpsd_ctx *ctx);

#endif
