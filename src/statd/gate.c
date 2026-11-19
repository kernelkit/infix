/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * sysrepo keeps a file per module for as long as a client subscribes
 * to changes in its operational datastore.
 */

#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
#include <sys/inotify.h>

#include <ev.h>
#include <sysrepo.h>
#include <libite/lite.h>
#include <srx/common.h>

#include "push.h"

static const char *push_keys[] = PUSH_KEYS;
static ev_io gate_io;
static int   gate_open = -1;

static int subscribed(void)
{
	char path[256];
	size_t i;

	for (i = 0; i < NELEMS(push_keys); i++) {
		snprintf(path, sizeof(path), "%s/%ssub_%.*s.operational", sr_get_shm_path(),
			 sr_get_shm_prefix(), (int)strcspn(push_keys[i], ":"), push_keys[i]);
		if (!access(path, F_OK))
			return 1;
	}

	return 0;
}

static void gate_update(void)
{
	int open = subscribed();

	if (open == gate_open)
		return;

	if (systemf("initctl -nbq cond %s %s", open ? "set" : "clear", PUSH_COND)) {
		ERROR("push: failed to %s condition %s", open ? "set" : "clear", PUSH_COND);
		return;
	}
	gate_open = open;
}

static void gate_cb(struct ev_loop *, ev_io *w, int)
{
	char buf[4096];

	while (read(w->fd, buf, sizeof(buf)) > 0)
		;
	gate_update();
}

int push_gate_init(struct ev_loop *loop)
{
	int fd;

	fd = inotify_init1(IN_NONBLOCK | IN_CLOEXEC);
	if (fd < 0 || inotify_add_watch(fd, sr_get_shm_path(), IN_CREATE | IN_DELETE) < 0) {
		ERROR("push: cannot watch %s: %s", sr_get_shm_path(), strerror(errno));
		if (fd >= 0)
			close(fd);
		return -1;
	}

	ev_io_init(&gate_io, gate_cb, fd, EV_READ);
	ev_io_start(loop, &gate_io);
	gate_update();

	return 0;
}
