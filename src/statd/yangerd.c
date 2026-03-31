/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/un.h>

#include <jansson.h>
#include <srx/common.h>

#include "yangerd.h"

static const char *yangerd_socket_path(void)
{
	const char *env;

	env = getenv("YANGERD_SOCKET");
	if (env && *env)
		return env;

	return YANGERD_SOCKET_DEFAULT;
}

static int yangerd_connect(void)
{
	struct sockaddr_un addr = { .sun_family = AF_UNIX };
	struct timeval tv = { .tv_sec = YANGERD_TIMEOUT_SEC };
	const char *path;
	int fd;

	path = yangerd_socket_path();
	if (strlen(path) >= sizeof(addr.sun_path)) {
		ERROR("yangerd socket path too long: %s", path);
		return -1;
	}
	strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

	fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
	if (fd < 0) {
		ERROR("yangerd: socket(): %s", strerror(errno));
		return -1;
	}

	setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));
	setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &tv, sizeof(tv));

	if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
		DEBUG("yangerd: connect(%s): %s", path, strerror(errno));
		close(fd);
		return -1;
	}

	return fd;
}

static int yangerd_write_all(int fd, const void *buf, size_t len)
{
	const unsigned char *p = buf;

	while (len > 0) {
		ssize_t n = write(fd, p, len);

		if (n < 0) {
			if (errno == EINTR)
				continue;
			return -1;
		}
		p += n;
		len -= n;
	}

	return 0;
}

static int yangerd_read_all(int fd, void *buf, size_t len)
{
	unsigned char *p = buf;

	while (len > 0) {
		ssize_t n = read(fd, p, len);

		if (n < 0) {
			if (errno == EINTR)
				continue;
			return -1;
		}
		if (n == 0) {
			errno = ECONNRESET;
			return -1;
		}
		p += n;
		len -= n;
	}

	return 0;
}

static int yangerd_send_request(int fd, const char *path)
{
	json_t *req;
	char *json_str;
	size_t json_len;
	unsigned char hdr[5];
	int rc = -1;

	req = json_pack("{s:s, s:s}", "method", "get", "path", path);
	if (!req)
		return -1;

	json_str = json_dumps(req, JSON_COMPACT);
	json_decref(req);
	if (!json_str)
		return -1;

	json_len = strlen(json_str);
	if (json_len > YANGERD_MAX_PAYLOAD) {
		free(json_str);
		return -1;
	}

	hdr[0] = YANGERD_PROTO_VERSION;
	hdr[1] = (json_len >> 24) & 0xff;
	hdr[2] = (json_len >> 16) & 0xff;
	hdr[3] = (json_len >>  8) & 0xff;
	hdr[4] = (json_len >>  0) & 0xff;

	if (yangerd_write_all(fd, hdr, sizeof(hdr)) < 0)
		goto out;
	if (yangerd_write_all(fd, json_str, json_len) < 0)
		goto out;

	rc = 0;
out:
	free(json_str);
	return rc;
}

static int yangerd_recv_response(int fd, char **buf, size_t *len)
{
	unsigned char hdr[5];
	uint32_t payload_len;
	json_error_t jerr;
	json_t *resp;
	json_t *status;
	json_t *data;
	char *body;
	char *data_str;

	*buf = NULL;
	*len = 0;

	if (yangerd_read_all(fd, hdr, sizeof(hdr)) < 0)
		return -1;

	if (hdr[0] != YANGERD_PROTO_VERSION) {
		ERROR("yangerd: protocol version mismatch: got %u, want %u",
		      hdr[0], YANGERD_PROTO_VERSION);
		return -1;
	}

	payload_len = ((uint32_t)hdr[1] << 24) |
		      ((uint32_t)hdr[2] << 16) |
		      ((uint32_t)hdr[3] <<  8) |
		      ((uint32_t)hdr[4]);

	if (payload_len > YANGERD_MAX_PAYLOAD) {
		ERROR("yangerd: payload too large: %u", payload_len);
		return -1;
	}

	body = malloc(payload_len + 1);
	if (!body)
		return -1;

	if (yangerd_read_all(fd, body, payload_len) < 0) {
		free(body);
		return -1;
	}
	body[payload_len] = '\0';

	resp = json_loads(body, 0, &jerr);
	free(body);
	if (!resp) {
		ERROR("yangerd: invalid response JSON: %s", jerr.text);
		return -1;
	}

	status = json_object_get(resp, "status");
	if (!json_is_string(status) || strcmp(json_string_value(status), "ok")) {
		json_t *msg = json_object_get(resp, "message");

		ERROR("yangerd: request failed: %s",
		      json_is_string(msg) ? json_string_value(msg) : "unknown");
		json_decref(resp);
		return -1;
	}

	data = json_object_get(resp, "data");
	if (!data || json_is_null(data)) {
		json_decref(resp);
		*buf = strdup("{}");
		*len = 2;
		return 0;
	}

	data_str = json_dumps(data, JSON_COMPACT);
	json_decref(resp);
	if (!data_str)
		return -1;

	*buf = data_str;
	*len = strlen(data_str);

	return 0;
}

int yangerd_query(const char *path, char **buf, size_t *len)
{
	int fd;
	int rc;

	*buf = NULL;
	*len = 0;

	fd = yangerd_connect();
	if (fd < 0)
		return -1;

	if (yangerd_send_request(fd, path) < 0) {
		ERROR("yangerd: failed sending request for %s", path);
		close(fd);
		return -1;
	}

	rc = yangerd_recv_response(fd, buf, len);
	if (rc < 0)
		ERROR("yangerd: failed reading response for %s", path);

	close(fd);

	return rc;
}
