/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_YANGERD_H_
#define STATD_YANGERD_H_

#include <stddef.h>

#define YANGERD_SOCKET_DEFAULT "/run/yangerd.sock"
#define YANGERD_TIMEOUT_SEC    5
#define YANGERD_MAX_PAYLOAD    (4 << 20) /* 4 MiB, matches Go side */
#define YANGERD_PROTO_VERSION  0x01

/**
 * yangerd_query() - Query yangerd daemon for operational YANG data
 * @path:   YANG model path, e.g. "ietf-interfaces:interfaces"
 * @buf:    Output pointer to malloc'd JSON string (caller must free)
 * @len:    Output length of JSON data
 *
 * Connects to the yangerd Unix socket, sends a "get" request for @path,
 * reads the framed response, and extracts the "data" field as a JSON
 * string.  The socket path defaults to %YANGERD_SOCKET_DEFAULT but can
 * be overridden with the YANGERD_SOCKET environment variable.
 *
 * Return: 0 on success, -1 on error (buf is set to NULL).
 */
int yangerd_query(const char *path, char **buf, size_t *len);

#endif
