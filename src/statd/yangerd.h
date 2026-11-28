/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_YANGERD_H_
#define STATD_YANGERD_H_

#include <stddef.h>
#include <libyang/libyang.h>

#define YANGERD_SOCKET_DEFAULT "/run/yangerd.sock"
#define YANGERD_TIMEOUT_SEC    5
#define YANGERD_MAX_PAYLOAD    (4 << 20) /* 4 MiB, matches Go side */
#define YANGERD_PROTO_VERSION  0x02

/**
 * yangerd_query() - Query yangerd daemon for operational YANG data
 * @path: YANG model path, e.g. "ietf-interfaces:interfaces"
 * @buf:  Output pointer to malloc'd JSON string (caller must free)
 * @len:  Output length of JSON data
 *
 * Connects to the yangerd Unix socket, sends a "get" request for @path,
 * and returns the data frame of the response.  The socket path defaults
 * to %YANGERD_SOCKET_DEFAULT but can be overridden with the
 * YANGERD_SOCKET environment variable.
 *
 * Return: 0 on success, 1 when yangerd is not running or still starting
 * (no data yet), -1 on error.  buf is NULL unless 0 is returned.
 */
int yangerd_query(const char *path, char **buf, size_t *len);

/* 0 with the parsed data or NULL in tree, -1 when json is not valid for the models */
int yangerd_parse(const struct ly_ctx *ctx, const char *key, const char *json, struct lyd_node **tree);

/* 0 with the parsed answer or NULL in tree, 1 when yangerd is not ready, -1 on error */
int yangerd_tree(const struct ly_ctx *ctx, const char *key, struct lyd_node **tree);

/* Returns a socket that is readable when one of keys has changed, or -1 */
int yangerd_watch(const char **keys, size_t num);

/* Reads one changed key and its stored data from a yangerd_watch() socket */
int yangerd_watch_read(int fd, char **key, char **json);

#endif
