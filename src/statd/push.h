/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef STATD_PUSH_H_
#define STATD_PUSH_H_

#include <ev.h>

/* Finit condition that statd-push runs on */
#define PUSH_COND "event-push"

/* yangerd keys that statd-push writes to operational */
#define PUSH_KEYS {				\
	"ietf-interfaces:interfaces",		\
	"ieee802-dot1ab-lldp:lldp",		\
	"ietf-alarms:alarms",			\
}

/* Sets PUSH_COND while a client subscribes to changes of a pushed module */
int push_gate_init(struct ev_loop *loop);

#endif
