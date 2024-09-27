/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_DAGGER_H_
#define CONFD_DAGGER_H_

#include <limits.h>
#include <stdio.h>
#include "core.h"

struct dagger {
	sr_session_ctx_t *session;

	int current, next;
	FILE *next_fp;

	char path[PATH_MAX];
};

FILE *dagger_fopen_next(struct dagger *d, const char *action, const char *node,
			unsigned char prio, const char *script);
FILE *dagger_fopen_current(struct dagger *d, const char *action, const char *node,
			   unsigned char prio, const char *script);

int dagger_add_dep(struct dagger *d, const char *depender, const char *dependee);
int dagger_add_node(struct dagger *d, const char *node);
int dagger_abandon(struct dagger *d);
int dagger_evolve(struct dagger *d);
int dagger_evolve_or_abandon(struct dagger *d);

void dagger_skip_iface(struct dagger *d, const char *ifname);
void dagger_skip_current_iface(struct dagger *d, const char *ifname);
int dagger_should_skip(struct dagger *d, const char *ifname);
int dagger_should_skip_current(struct dagger *d, const char *ifname);
int dagger_is_bootstrap(struct dagger *d);

int dagger_claim(struct dagger *d, const char *path);

#endif	/* CONFD_DAGGER_H_ */
