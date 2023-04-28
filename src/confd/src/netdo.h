/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_NETDO_H_
#define CONFD_NETDO_H_

#include <stdio.h>

#include <net/if.h>

#define NETDO_PHASES(_op) \
	_op(INIT, "init") \
	_op(EXIT, "exit")

enum netdo_phase {
#define netdo_phase_enum_gen(_ph, ...) \
	NETDO_ ## _ph,

	NETDO_PHASES(netdo_phase_enum_gen)
#undef netdo_phase_enum_gen

	__NETDO_PHASE_MAX
};

#define NETDO_CMDS(_op)			\
	_op(BRIDGE, ".bridge")		\
	_op(ETHTOOL, "-ethtool.sh")	\
	_op(IP, ".ip")

enum netdo_cmd {
#define netdo_cmd_enum_gen(_cmd, ...) \
	NETDO_ ## _cmd,

	NETDO_CMDS(netdo_cmd_enum_gen)
#undef netdo_cmd_enum_gen

	__NETDO_CMD_MAX
};

struct netdo_iface {
	TAILQ_ENTRY(netdo_iface) node;

	char name[IFNAMSIZ];
	FILE *cmd[__NETDO_PHASE_MAX][__NETDO_CMD_MAX];
};

TAILQ_HEAD(netdo_ifaces, netdo_iface);

struct netdo {
	FILE *next_fp;
	int gen;
	struct netdo_ifaces ifaces;

	char path[__NETDO_PHASE_MAX][PATH_MAX];
};

FILE *netdo_get_file(struct netdo *nd, const char *ifname,
		     enum netdo_phase phase, enum netdo_cmd cmd);

int netdo_change(struct netdo *nd, struct lyd_node *cifs, struct lyd_node *difs);
int netdo_done(struct netdo *nd);
int netdo_abort(struct netdo *nd);

int netdo_boot(void);


#endif /* CONFD_NETDO_H_ */
