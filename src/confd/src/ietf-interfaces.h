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

#define IFTYPES(_map) \
	_map(IFT_BRIDGE, "infix-if-type:bridge")	\
	_map(IFT_DUMMY,  "infix-if-type:dummy")		\
	_map(IFT_ETH,    "infix-if-type:ethernet")	\
	_map(IFT_ETHISH, "infix-if-type:etherlike")	\
	_map(IFT_GRE,    "infix-if-type:gre")		\
	_map(IFT_GRETAP, "infix-if-type:gretap")	\
	_map(IFT_VETH,   "infix-if-type:veth")		\
	_map(IFT_VLAN,   "infix-if-type:vlan")		\
	_map(IFT_VXLAN,  "infix-if-type:vxlan")		\
	/*  */

enum iftype {
#define ift_enum(_enum, _str) _enum,
	IFTYPES(ift_enum)
#undef ift_enum

	IFT_UNKNOWN
};

static inline enum iftype iftype_from_str(const char *typestr)
{
#define ift_cmp(_enum, _str) if (!strcmp(typestr, _str)) return _enum;
	IFTYPES(ift_cmp)
#undef ift_cmp
	return IFT_UNKNOWN;
}

static inline enum iftype iftype_from_iface(struct lyd_node *ifn)
{
	const char *typestr = lydx_get_cattr(ifn, "type");

	if (!typestr)
		return IFT_UNKNOWN;

	return iftype_from_str(typestr);
}

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
const char *get_chassis_addr(void);
int link_gen_address(struct lyd_node *cif, FILE *ip);

/* ietf-ip.c */
int netdag_gen_ipv6_autoconf(struct dagger *net, struct lyd_node *cif,
			     struct lyd_node *dif, FILE *ip);
int netdag_gen_ipv4_autoconf(struct dagger *net, struct lyd_node *cif,
			     struct lyd_node *dif);
int netdag_gen_ip_addrs(struct dagger *net, FILE *ip, const char *proto,
			struct lyd_node *cif, struct lyd_node *dif);

/* infix-if-bridge.c */
int bridge_mstpd_gen(struct lyd_node *cifs);
int bridge_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip, int add);
int bridge_add_deps(struct lyd_node *cif);
/* infix-if-bridge-mcd.c */
int bridge_mcd_gen(struct lyd_node *cifs);
/* infix-if-bridge-port.c */
int bridge_port_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip);

/* infix-if-gre.c */
int gre_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip);

/* infix-if-veth.c */
bool veth_is_primary(struct lyd_node *cif);
int ifchange_cand_infer_veth(sr_session_ctx_t *session, const char *path);
int veth_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip);
int veth_add_deps(struct lyd_node *cif);

/* infix-if-vlan.c */
int ifchange_cand_infer_vlan(sr_session_ctx_t *session, const char *path);
int vlan_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip);
int vlan_add_deps(struct lyd_node *cif);

/* infix-if-vxlan.c */
int vxlan_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip);

#endif /* CONFD_IETF_INTERFACES_H_ */
