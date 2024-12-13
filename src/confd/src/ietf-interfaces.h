/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_IETF_INTERFACES_H_
#define CONFD_IETF_INTERFACES_H_

#include "core.h"
#include "cni.h"

#define IF_XPATH      "/ietf-interfaces:interfaces/interface"
#define IF_VLAN_XPATH "%s/infix-interfaces:vlan"

#define ERR_IFACE(_iface, _err, _fmt, ...)				\
	({								\
		ERROR("%s: " _fmt, lydx_get_cattr(_iface, "name"),	\
		      ##__VA_ARGS__);					\
		_err;							\
	})

#define DEBUG_IFACE(_iface, _fmt, ...)				\
	DEBUG("%s: " _fmt, lydx_get_cattr(_iface, "name"), ##__VA_ARGS__)

#define ONOFF(boolean) boolean ? "on" : "off"

static inline const char *bridge_tagtype2str(const char *type)
{
	if (!strcmp(type, "ieee802-dot1q-types:c-vlan"))
		return "802.1Q";
	else if (!strcmp(type, "ieee802-dot1q-types:s-vlan"))
		return "802.1ad";

	return NULL;
}

static inline bool is_bridge_port(struct lyd_node *cif)
{
	struct lyd_node *node = lydx_get_descendant(lyd_child(cif), "bridge-port", NULL);

	if (!node || !lydx_get_child(node, "bridge"))
		return false;

	return true;
}


/* ieee802-ethernet-interface.c */
int netdag_gen_ethtool(struct dagger *net, struct lyd_node *cif, struct lyd_node *dif);

/* ietf-interfaces.c */
char *get_phys_addr(struct lyd_node *parent, int *deleted);
int netdag_exit_reload(struct dagger *net);

/* ietf-ip.c */
int netdag_gen_ipv6_autoconf(struct dagger *net, struct lyd_node *cif,
			     struct lyd_node *dif, FILE *ip);
int netdag_gen_ipv4_autoconf(struct dagger *net, struct lyd_node *cif,
			     struct lyd_node *dif);
int netdag_gen_ip_addrs(struct dagger *net, FILE *ip, const char *proto,
			struct lyd_node *cif, struct lyd_node *dif);

/* infix-if-bridge.c */
void mcast_querier(const char *ifname, int vid, int mode, int interval);
int bridge_gen_ports(struct dagger *net, struct lyd_node *dif, struct lyd_node *cif, FILE *ip);
int netdag_gen_bridge(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *dif,
		      struct lyd_node *cif, FILE *ip, int add);

/* infix-if-veth.c */
int ifchange_cand_infer_veth(sr_session_ctx_t *session, const char *path);
int netdag_gen_veth(struct dagger *net, struct lyd_node *dif,
		    struct lyd_node *cif, FILE *ip);

/* infix-if-vlan.c */
int ifchange_cand_infer_vlan(sr_session_ctx_t *session, const char *path);
int netdag_gen_vlan(struct dagger *net, struct lyd_node *dif,
		    struct lyd_node *cif, FILE *ip);

#endif /* CONFD_IETF_INTERFACES_H_ */


