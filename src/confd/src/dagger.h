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


enum netdag_exit {
	/* Stop daemons running on interface (zcip etc.) */
	NETDAG_EXIT_DAEMON = 30,

	/* Interface specific tear-down (bridge objects) of lowers */
	NETDAG_EXIT_LOWERS_PROTO = 35,

	/* Interface specific tear-down (bridge objects) */
	NETDAG_EXIT_PROTO = 40,

	/* Detach lower interfaces (bridge ports, LAG ports, etc.) */
	NETDAG_EXIT_LOWERS = 45,

	/* Tear down interface settings, remove virtual interfaces */
	NETDAG_EXIT_PRE = 49,
	NETDAG_EXIT = 50,
	NETDAG_EXIT_POST = 51,
};

enum netdag_init {
	/* Configure link layer */
	NETDAG_INIT_PHYS = 10,

	/* Configure interface settings, create virtual interfaces */
	NETDAG_INIT_PRE = 49,
	NETDAG_INIT = 50,
	NETDAG_INIT_POST = 51,

	/* Attach lower interfaces (bridge ports, LAG ports, etc.) */
	NETDAG_INIT_LOWERS = 55,

	/* Interface specific setup (bridge VLANs, MDB entries, etc.) */
	NETDAG_INIT_PROTO = 60,

	/* Interface specific setup (bridge VLAN PVID) of lowers */
	NETDAG_INIT_LOWERS_PROTO = 65,

	/* Start/configure daemons on interface (mstpd, zcip etc.) */
	NETDAG_INIT_DAEMON = 70,

	/* Inject lower-specific configuration to daemon (mstpd) */
	NETDAG_INIT_DAEMON_LOWERS = 75,
};

FILE *dagger_fopen_net_init(struct dagger *d, const char *node, enum netdag_init order,
			    const char *script);
FILE *dagger_fopen_net_exit(struct dagger *d, const char *node, enum netdag_exit order,
			    const char *script);

#endif	/* CONFD_DAGGER_H_ */
