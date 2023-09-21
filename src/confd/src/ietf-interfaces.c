/* SPDX-License-Identifier: BSD-3-Clause */

#include <fnmatch.h>
#include <stdbool.h>
#include <jansson.h>
#include <arpa/inet.h>
#include <net/if.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_module.h>
#include <srx/srx_val.h>

#include "core.h"
#include "dagger.h"

#define ERR_IFACE(_iface, _err, _fmt, ...)				\
	({								\
		ERROR("%s: " _fmt, lydx_get_cattr(_iface, "name"),	\
		      ##__VA_ARGS__);					\
		_err;							\
	})

#define DEBUG_IFACE(_iface, _fmt, ...)				\
	DEBUG("%s: " _fmt, lydx_get_cattr(_iface, "name"), ##__VA_ARGS__)

#define IF_XPATH "/ietf-interfaces:interfaces/interface"

static bool iface_is_phys(const char *ifname)
{
	bool is_phys = false;
	json_error_t jerr;
	const char *attr;
	json_t *link;
	FILE *proc;

	proc = popenf("re", "ip -d -j link show dev %s 2>/dev/null", ifname);
	if (!proc)
		goto out;

	link = json_loadf(proc, 0, &jerr);
	pclose(proc);

	if (!link)
		goto out;

	if (json_unpack(link, "[{s:s}]", "link_type", &attr))
		goto out_free;

	if (strcmp(attr, "ether"))
		goto out_free;

	if (json_unpack(link, "[{s:s}]", "parentbus", &attr))
		goto out_free;

	is_phys = true;

out_free:
	json_decref(link);
out:
	return is_phys;
}

/*
 * Needed because we do deep searches for changes in interfaces,
 * e.g. changes in bridge port settings, veth peers, etc.
 */
static char *iface_xpath(const char *xpath)
{
	char *path, *ptr;

	path = strdup(xpath);
	if (!path)
		return NULL;
	if (!(ptr = strstr(path, "]/"))) {
		free(path);
		return NULL;
	}
	ptr[1] = 0;

	return path;
}

static int ifchange_cand_infer_veth(sr_session_ctx_t *session, const char *path)
{
	char *ifname, *type, *peer, *xpath, *val;
	sr_error_t err = SR_ERR_OK;
	size_t cnt = 0;

	xpath = iface_xpath(path);
	if (!xpath)
		return SR_ERR_SYS;

	type = srx_get_str(session, "%s/type", xpath);
	if (!type || strcmp(type, "infix-if-type:veth"))
		goto out;

	ifname = srx_get_str(session, "%s/name", xpath);
	if (!ifname)
		goto out_free_type;

	peer = srx_get_str(session, "%s/veth/peer", xpath);
	if (!peer)
		goto out_free_ifname;

	err = srx_nitems(session, &cnt, "/interfaces/interface[name='%s']/name", peer);
	if (err || cnt)
		goto out_free_peer;

	val = "infix-if-type:veth";
	err = srx_set_str(session, val, 0, IF_XPATH "[name='%s']/type", peer);
	if (err) {
		ERROR("failed setting iface %s type %s, err %d", peer, val, err);
		goto out_free_peer;
	}

	err = srx_set_str(session, ifname, 0, IF_XPATH "[name='%s']/infix-interfaces:veth/peer", peer);
	if (err)
		ERROR("failed setting iface %s peer %s, err %d", peer, ifname, err);

out_free_peer:
	free(peer);
out_free_ifname:
	free(ifname);
out_free_type:
	free(type);
out:
	free(xpath);
	return err;
}

static int ifchange_cand_infer_vlan(sr_session_ctx_t *session, const char *path)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	char *ifname, *type, *vidstr, *xpath;
	sr_error_t err = SR_ERR_OK;
	size_t cnt = 0;
	long vid;

	xpath = iface_xpath(path);
	if (!xpath)
		return SR_ERR_SYS;;
	type = srx_get_str(session, "%s/type", xpath);
	if (!type)
		goto out;
	if (strcmp(type, "infix-if-type:vlan"))
		goto out_free_type;

	ifname = srx_get_str(session, "%s/name", xpath);
	if (!ifname)
		goto out_free_type;

	if (fnmatch("*.+([0-9])", ifname, FNM_EXTMATCH))
		goto out_free_ifname;

	vidstr = rindex(ifname, '.');
	if (!vidstr)
		goto out_free_ifname;

	*vidstr++ = '\0';
	vid = strtol(vidstr, NULL, 10);
	if (vid < 1 || vid > 4095)
		goto out_free_ifname;

	err = srx_nitems(session, &cnt,
			 "/interfaces/interface[name='%s']/name", ifname);
	if (err || !cnt)
		goto out_free_ifname;

	err = srx_nitems(session, &cnt,
			 "%s/ietf-if-extensions:parent-interface", xpath);
	if (!err && !cnt) {
		inferred.data.string_val = ifname;
		err = srx_set_item(session, &inferred, 0,
				   "%s/ietf-if-extensions:parent-interface", xpath);
		if (err)
			goto out_free_ifname;
	}

	err = srx_nitems(session, &cnt,
			 "%s"
			 "/ietf-if-extensions:encapsulation"
			 "/ietf-if-vlan-encapsulation:dot1q-vlan"
			 "/outer-tag/tag-type", xpath);
	if (!err && !cnt) {
		inferred.data.string_val = "ieee802-dot1q-types:c-vlan";
		err = srx_set_item(session, &inferred, 0,
				   "%s"
				   "/ietf-if-extensions:encapsulation"
				   "/ietf-if-vlan-encapsulation:dot1q-vlan"
				   "/outer-tag/tag-type", xpath);
		if (err)
			goto out_free_ifname;
	}

	err = srx_nitems(session, &cnt,
			 "%s"
			 "/ietf-if-extensions:encapsulation"
			 "/ietf-if-vlan-encapsulation:dot1q-vlan"
			 "/outer-tag/vlan-id", xpath);
	if (!err && !cnt) {
		inferred.data.string_val = vidstr;
		err = srx_set_item(session, &inferred, 0,
				   "%s"
				   "/ietf-if-extensions:encapsulation"
				   "/ietf-if-vlan-encapsulation:dot1q-vlan"
				   "/outer-tag/vlan-id", xpath);
		if (err)
			goto out_free_ifname;
	}

out_free_ifname:
	free(ifname);
out_free_type:
	free(type);
out:
	free(xpath);
	return err;
}

static int ifchange_cand_infer_type(sr_session_ctx_t *session, const char *path)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	char *ifname, *type, *xpath;
	sr_error_t err = SR_ERR_OK;

	xpath = iface_xpath(path);
	if (!path)
		return SR_ERR_SYS;

	type = srx_get_str(session, "%s/type", xpath);
	if (type) {
		free(type);
		goto out;
	}

	ifname = srx_get_str(session, "%s/name", xpath);
	if (!ifname) {
		err = SR_ERR_INTERNAL;
		goto out;
	}

	if (iface_is_phys(ifname))
		inferred.data.string_val = "infix-if-type:ethernet";
	else if (!fnmatch("br+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:bridge";
	else if (!fnmatch("lag+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:lag";
	else if (!fnmatch("veth+([0-9a-z_-])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:veth";
	else if (!fnmatch("vlan+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:vlan";
	else if (!fnmatch("*.+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:vlan";

	free(ifname);

	if (inferred.data.string_val)
		err = srx_set_item(session, &inferred, 0, "%s/type", xpath);

out:
	free(xpath);
	return err;
}

static int ifchange_cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	sr_change_iter_t *iter;
	sr_change_oper_t op;
	sr_val_t *old, *new;
	sr_error_t err;

	switch (event) {
	case SR_EV_UPDATE:
	case SR_EV_CHANGE:
		break;
	default:
		return SR_ERR_OK;
	}

	err = sr_dup_changes_iter(session, "/ietf-interfaces:interfaces/interface//*", &iter);
	if (err)
		return err;

	while (sr_get_change_next(session, iter, &op, &old, &new) == SR_ERR_OK) {
		switch (op) {
		case SR_OP_CREATED:
		case SR_OP_MODIFIED:
			break;
		default:
			continue;
		}

		err = ifchange_cand_infer_type(session, new->xpath);
		if (err)
			break;

		err = ifchange_cand_infer_veth(session, new->xpath);
		if (err)
			break;

		err = ifchange_cand_infer_vlan(session, new->xpath);
		if (err)
			break;
	}

	sr_free_change_iter(iter);
	return SR_ERR_OK;
}

static int netdag_exit_reload(struct dagger *net)
{
	FILE *initctl;

	/* We may end up writing this file multiple times, e.g. if
	 * multiple services are disabled in the same config cycle,
	 * but since the contents of the file are static it doesn't
	 * matter.
	 */
	initctl = dagger_fopen_current(net, "exit", "@post",
				       90, "reload.sh");
	if (!initctl)
		return -EIO;

	fputs("initctl -bnq reload\n", initctl);
	fclose(initctl);
	return 0;
}

static bool is_std_lo_addr(const char *ifname, const char *ip, const char *pf)
{
	struct in6_addr in6, lo6;
	struct in_addr in4;

	if (strcmp(ifname, "lo"))
		return false;

	if (inet_pton(AF_INET, ip, &in4) == 1)
		return (ntohl(in4.s_addr) == INADDR_LOOPBACK) && !strcmp(pf, "8");

	if (inet_pton(AF_INET6, ip, &in6) == 1) {
		inet_pton(AF_INET6, "::1", &lo6);

		return !memcmp(&in6, &lo6, sizeof(in6))
			&& !strcmp(pf, "128");
	}

	return false;
}

static int netdag_gen_diff_addr(FILE *ip, const char *ifname,
				struct lyd_node *addr)
{
	enum lydx_op op = lydx_get_op(addr);
	struct lyd_node *adr, *pfx;
	struct lydx_diff adrd, pfxd;
	const char *addcmd = "add";

	adr = lydx_get_child(addr, "ip");
	pfx = lydx_get_child(addr, "prefix-length");
	if (!adr || !pfx)
		return -EINVAL;

	lydx_get_diff(adr, &adrd);
	lydx_get_diff(pfx, &pfxd);

	if (op != LYDX_OP_CREATE) {
		fprintf(ip, "address delete %s/%s dev %s\n",
			adrd.old, pfxd.old, ifname);

		if (op == LYDX_OP_DELETE)
			return 0;
	}

	/* When bringing up loopback, the kernel will automatically
	 * add the standard addresses, so don't treat the existance of
	 * these as an error.
	 */
	if ((op == LYDX_OP_CREATE) &&
	    is_std_lo_addr(ifname, adrd.new, pfxd.new))
		addcmd = "replace";

	fprintf(ip, "address %s %s/%s dev %s proto 4\n", addcmd,
		adrd.new, pfxd.new, ifname);
	return 0;
}

static int netdag_gen_diff_addrs(FILE *ip, const char *ifname,
				 struct lyd_node *ipvx)
{
	struct lyd_node *addr;
	int err = 0;

	LYX_LIST_FOR_EACH(lyd_child(ipvx), addr, "address") {
		err = netdag_gen_diff_addr(ip, ifname, addr);
		if (err)
			break;
	}

	return err;
}

static int netdag_set_conf_addrs(FILE *ip, const char *ifname,
				 struct lyd_node *ipvx)
{
	struct lyd_node *addr;

	LYX_LIST_FOR_EACH(lyd_child(ipvx), addr, "address") {
		fprintf(ip, "address add %s/%s dev %s\n",
			lydx_get_cattr(addr, "ip"),
			lydx_get_cattr(addr, "prefix-length"),
			ifname);
	}

	return 0;
}

static int netdag_gen_link_addr(FILE *ip, struct lyd_node *cif, struct lyd_node *dif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	const char *mac = NULL;
	struct lyd_node *node;
	char buf[32];

	node = lydx_get_child(dif, "phys-address");
	if (lydx_get_op(node) == LYDX_OP_DELETE) {
		FILE *fp;

		fp = popenf("r", "ethtool -P %s |awk '{print $3}'", ifname);
		if (fp) {
			if (fgets(buf, sizeof(buf), fp))
				mac = chomp(buf);
			pclose(fp);
		}
	} else {
		mac = lyd_get_value(node);
	}

	if (!mac || !strlen(mac)) {
		DEBUG("No change in %s phys-address, skipping ...", ifname);
		return 0;
	}

	fprintf(ip, "link set %s address %s\n", ifname, mac);
	return 0;
}

static int netdag_gen_ip_addrs(FILE *ip, const char *proto,
	struct lyd_node *cif, struct lyd_node *dif)
{
	struct lyd_node *ipconf = lydx_get_child(cif, proto);
	struct lyd_node *ipdiff = lydx_get_child(dif, proto);
	const char *ifname = lydx_get_cattr(dif, "name");

	if (!ipconf || !lydx_is_enabled(ipconf, "enabled")) {
		if (if_nametoindex(ifname))
			systemf("ip -%c addr flush dev %s\n", proto[3], ifname);
		return 0;
	}

	if (lydx_get_op(lydx_get_child(ipdiff, "enabled")) == LYDX_OP_REPLACE)
		return netdag_set_conf_addrs(ip, ifname, ipconf);

	return netdag_gen_diff_addrs(ip, ifname, ipdiff);
}

static int netdag_gen_ipv6_autoconf(struct dagger *net, struct lyd_node *cif,
				    struct lyd_node *dif, FILE *ip)
{
	const char *preferred_lft = "86400", *valid_lft = "604800";
	struct lyd_node *ipconf = lydx_get_child(cif, "ipv6");
	const char *ifname = lydx_get_cattr(dif, "name");
	int global = 0, random = 0;
	struct lyd_node *node;
	FILE *fp;

	if (!ipconf || !lydx_is_enabled(ipconf, "enabled")) {
		fputs(" addrgenmode none", ip);
		return 0;
	}

	node = lydx_get_child(ipconf, "autoconf");
	if (node) {
		global = lydx_is_enabled(node, "create-global-addresses");
		random = lydx_is_enabled(node, "create-temporary-addresses");

		preferred_lft = lydx_get_cattr(node, "temporary-preferred-lifetime");
		valid_lft     = lydx_get_cattr(node, "temporary-valid-lifetime");
	}

	fp = dagger_fopen_next(net, "init", ifname, 45, "init.sysctl");
	if (fp) {
		/* Autoconfigure addresses using Prefix Information in Router Advertisements */
		fprintf(fp, "net.ipv6.conf.%s.autoconf = %d\n", ifname, global);
		/* The amount of Duplicate Address Detection probes to send. */
		fprintf(fp, "net.ipv6.conf.%s.dad_transmits = %s\n", ifname,
			lydx_get_cattr(ipconf, "dup-addr-detect-transmits"));
		/* Preferred and valid lifetimes for temporary (random) addresses */
		fprintf(fp, "net.ipv6.conf.%s.temp_prefered_lft = %s\n", ifname, preferred_lft);
		fprintf(fp, "net.ipv6.conf.%s.temp_valid_lft = %s\n", ifname, valid_lft);
		fclose(fp);
	}

	fprintf(ip, " addrgenmode %s", random ? "random" : "eui64");

	return 0;
}

static int netdag_gen_ipv4_autoconf(struct dagger *net, struct lyd_node *cif,
				    struct lyd_node *dif)
{
	struct lyd_node *ipconf = lydx_get_child(cif, "ipv4");
	struct lyd_node *ipdiff = lydx_get_child(dif, "ipv4");
	const char *ifname = lydx_get_cattr(dif, "name");
	struct lyd_node *node, *zcip;
	struct lydx_diff nd;
	FILE *initctl;
	int err = 0;

	if (!ipconf || !lydx_is_enabled(ipconf, "enabled"))
		goto disable;

	if (lydx_get_op(lydx_get_child(ipdiff, "enabled")) == LYDX_OP_REPLACE) {
		node = lydx_get_child(ipconf, "autoconf");
		if (node && lydx_is_enabled(node, "enabled"))
			goto enable;
		goto disable;
	}

	zcip = lydx_get_child(ipdiff, "autoconf");
	if (!zcip || !(node = lydx_get_child(zcip, "enabled")))
		return 0;

	lydx_get_diff(node, &nd);
	if (nd.new && !strcmp(nd.val, "true")) {
	enable:
		initctl = dagger_fopen_next(net, "init", ifname,
					    60, "zeroconf-up.sh");
		if (!initctl)
			return -EIO;

		fprintf(initctl,
			"initctl -bnq enable zeroconf@%s.conf\n", ifname);
	} else {
	disable:
		initctl = dagger_fopen_current(net, "exit", ifname,
					       40, "zeroconf-down.sh");
		if (!initctl) {
			/* check if in bootstrap (pre gen 0) */
			if (errno == EUNATCH)
				return 0;
			return -EIO;
		}

		fprintf(initctl,
			"initctl -bnq disable zeroconf@%s.conf\n", ifname);

		err = netdag_exit_reload(net);
	}

	fclose(initctl);
	return err;
}

static int netdag_gen_sysctl_bool(struct dagger *net,
				  const char *ifname, FILE **fpp,
				  struct lyd_node *node,
				  const char *fmt, ...)
{
	struct lydx_diff nd;
	va_list ap;

	if (!node)
		return 0;

	if (!lydx_get_diff(node, &nd))
		return 0;

	*fpp = *fpp ? : dagger_fopen_next(net, "init", ifname,
					  60, "init.sysctl");
	if (!*fpp)
		return -EIO;

	va_start(ap, fmt);
	vfprintf(*fpp, fmt, ap);
	va_end(ap);
	fprintf(*fpp, " = %u\n", (nd.new && !strcmp(nd.val, "true")) ? 1 : 0);
	return 0;
}
static int netdag_gen_sysctl(struct dagger *net,
			     struct lyd_node *dif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	struct lyd_node *node;
	FILE *sysctl = NULL;
	int err = 0;

	node = lydx_get_descendant(lyd_child(dif), "ipv4", "forwarding", NULL);
	err = err ? : netdag_gen_sysctl_bool(net, ifname, &sysctl, node,
					     "net.ipv4.conf.%s.forwarding",
					     ifname);

	node = lydx_get_descendant(lyd_child(dif), "ipv6", "forwarding", NULL);
	err = err ? : netdag_gen_sysctl_bool(net, ifname, &sysctl, node,
					     "net.ipv6.conf.%s.forwarding",
					     ifname);

	if (sysctl)
		fclose(sysctl);

	return err;
}

static int bridge_diff_vlan_port(struct dagger *net, FILE *br, const char *brname, int vid,
				       const char *brport, int tagged, enum lydx_op op)
{
	int pvid = 0;

	srx_get_int(net->session, &pvid, SR_UINT16_T, IF_XPATH "[name='%s']/bridge-port/pvid", brport);

	if (op != LYDX_OP_CREATE) {
		fprintf(br, "vlan del vid %d dev %s\n", vid, brport);
		if (op == LYDX_OP_DELETE)
			return 0;
	}

	fprintf(br, "vlan add vid %d dev %s %s %s %s\n", vid, brport, vid == pvid ? "pvid" : "",
		tagged ? "" : "untagged", strcmp(brname, brport) ? "" : "self");

	return 0;
}

static int bridge_diff_vlan_ports(struct dagger *net, FILE *br, const char *brname,
				   int vid, struct lyd_node *ports, int tagged)
{
	const char *type = tagged ? "tagged" : "untagged";
	struct lyd_node *port;
	int err = 0;

	LYX_LIST_FOR_EACH(lyd_child(ports), port, type) {
		const char *brport = lyd_get_value(port);
		enum lydx_op op = lydx_get_op(port);

		err = bridge_diff_vlan_port(net, br, brname, vid, brport, tagged, op);
		if (err)
			break;
	}

	return err;
}

static const char *bridge_tagtype2str(const char *type)
{
	const char *proto;

	if (!strcmp(type, "ieee802-dot1q-types:c-vlan"))
		proto = "802.1Q";
	else if (!strcmp(type, "ieee802-dot1q-types:s-vlan"))
		proto = "802.1ad";
	else
		proto = NULL;

	return proto;
}

static int bridge_vlan_settings(struct lyd_node *cif, const char **proto, int *pvid)
{
	struct lyd_node *node;

	node = lydx_get_descendant(lyd_child(cif), "bridge", "vlans", NULL);
	if (node) {
		const char *type = lydx_get_cattr(node, "proto");
		const char *vid = lydx_get_cattr(node, "pvid");

		if (!type || !vid) {
			ERROR("Missing bridge proto %s or pvid %s, defaulting to 1Q and 1.", type, vid);
			*proto = "802.1Q";
			*pvid = 1;
			return 1;
		}
		*pvid  = atoi(vid);
		*proto = bridge_tagtype2str(type);
		return 1;
	}

	return 0;
}

static int bridge_gen_ports(struct dagger *net, struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	struct lyd_node *node, *bridge;
	struct lydx_diff brdiff;
	const char *ifname;
	int err = 0;

	node = lydx_get_descendant(lyd_child(dif), "bridge-port", NULL);
	bridge = lydx_get_child(node, "bridge");
	if (!node || !bridge)
		return 0;	/* not a bridge port, skip */

	ifname = lydx_get_cattr(cif, "name");

	if (lydx_get_diff(bridge, &brdiff) && brdiff.old) {
		FILE *prev;

		prev = dagger_fopen_current(net, "exit", brdiff.old, 55, "exit.ip");
		if (!prev) {
			err = -EIO;
			goto fail;
		}
		fprintf(prev, "link set %s nomaster\n", ifname);
		fclose(prev);
	}

	if (brdiff.new) {
		FILE *next;

		next = dagger_fopen_next(net, "init", brdiff.new, 55, "init.ip");
		if (!next) {
			err = -EIO;
			goto fail;
		}
		fprintf(next, "link set %s master %s\n", ifname, brdiff.new);
		fclose(next);

		err = dagger_add_dep(net, brdiff.new, ifname);
		if (err)
			return ERR_IFACE(cif, err, "Unable to add dep \"%s\" to %s", ifname, brdiff.new);
	}
fail:
	return err;
}

static int bridge_fwd_mask(struct lyd_node *cif)
{
	struct lyd_node *node, *proto;
	int fwd_mask = 0;

	node = lydx_get_descendant(lyd_child(cif), "bridge", NULL);
	if (!node)
		goto fail;

	LYX_LIST_FOR_EACH(lyd_child(node), proto, "ieee-group-forward") {
		struct lyd_node_term  *leaf  = (struct lyd_node_term *)proto;
		struct lysc_node_leaf *sleaf = (struct lysc_node_leaf *)leaf->schema;

		if ((sleaf->nodetype & (LYS_LEAF | LYS_LEAFLIST)) && (sleaf->type->basetype == LY_TYPE_UNION)) {
			struct lyd_value *actual = &leaf->value.subvalue->value;
			int val;

			if (actual->realtype->basetype == LY_TYPE_UINT8)
				val = actual->uint8;
			else
				val = actual->enum_item->value;

			fwd_mask |= 1 << val;
		}
	}

fail:
	return fwd_mask;
}

static int netdag_gen_bridge(struct dagger *net, struct lyd_node *dif,
			     struct lyd_node *cif, FILE *ip, int add)
{
	const char *brname = lydx_get_cattr(cif, "name");
	const char *op = add ? "add" : "set";
	struct lyd_node *vlans, *vlan;
	int vlan_filtering, pvid, fwd_mask;
	const char *proto;
	FILE *br = NULL;
	int err = 0;

	vlan_filtering = bridge_vlan_settings(cif, &proto, &pvid);
	fwd_mask = bridge_fwd_mask(cif);

	fprintf(ip, "link %s dev %s type bridge group_fwd_mask %d vlan_filtering %d",
		op, brname, fwd_mask, vlan_filtering ? 1 : 0);
	if (!vlan_filtering) {
		fputc('\n', ip);
		goto done;
	} else if (!proto) {
		fputc('\n', ip);
		ERROR("%s: unsupported bridge proto", brname);
		err = -ENOSYS;
		goto done;
	}

	fprintf(ip, " vlan_protocol %s vlan_default_pvid %d\n", proto, pvid);

	vlans = lydx_get_descendant(lyd_child(dif), "bridge", "vlans", NULL);
	if (!vlans)
		goto done;

	br = dagger_fopen_next(net, "init", brname, 60, "init.bridge");
	if (!br) {
		err = -EIO;
		goto done;
	}

	LYX_LIST_FOR_EACH(lyd_child(vlans), vlan, "vlan") {
		int vid = atoi(lydx_get_cattr(vlan, "vid"));

		err = bridge_diff_vlan_ports(net, br, brname, vid, vlan, 0);
		if (err)
			break;

		err = bridge_diff_vlan_ports(net, br, brname, vid, vlan, 1);
		if (err)
			break;
	}

	fclose(br);
done:
	return err;
}

static int netdag_gen_veth(struct dagger *net, struct lyd_node *dif,
			   struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lyd_node *node;
	const char *peer;
	int err;

	node = lydx_get_descendant(lyd_child(cif), "veth", NULL);
	if (!node)
		return -EINVAL;

	peer = lydx_get_cattr(node, "peer");
	if (dagger_should_skip(net, ifname)) {
		err = dagger_add_dep(net, ifname, peer);
		if (err)
			return ERR_IFACE(cif, err, "Unable to add dep \"%s\" to %s",
					 peer, ifname);
	} else {
		dagger_skip_iface(net, peer);
		fprintf(ip, "link add dev %s down type veth peer %s\n", ifname, peer);
	}

	return 0;
}

static int netdag_gen_vlan(struct dagger *net, struct lyd_node *dif,
			   struct lyd_node *cif, FILE *ip)
{
	const char *parent = lydx_get_cattr(cif, "parent-interface");
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lydx_diff typed, vidd;
	struct lyd_node *otag;
	const char *proto;
	int err;

	DEBUG("ifname %s parent %s", ifname, parent);

	err = dagger_add_dep(net, ifname, parent);
	if (err)
		return ERR_IFACE(cif, err, "Unable to add dep \"%s\"", parent);

	otag = lydx_get_descendant(lyd_child(dif ? : cif),
				   "encapsulation",
				   "dot1q-vlan",
				   "outer-tag",
				   NULL);
	if (!otag) {
		/*
		 * Note: this is only an error if outer-tag is missing
		 * from cif, otherwise it just means the interface had a
		 * a change that was not related to the VLAN config.
		 */
		if (!dif)
			ERROR("%s: missing mandatory outer-tag", ifname);
		return 0;
	}

	fprintf(ip, "link add dev %s down link %s type vlan", ifname, parent);

	if (lydx_get_diff(lydx_get_child(otag, "tag-type"), &typed)) {
		proto = bridge_tagtype2str(typed.new);
		if (!proto)
			return ERR_IFACE(cif, -ENOSYS, "Unsupported tag type \"%s\"", typed.new);

		fprintf(ip, " proto %s", proto);
	}

	if (lydx_get_diff(lydx_get_child(otag, "vlan-id"), &vidd))
		fprintf(ip, " id %s", vidd.new);

	fputc('\n', ip);

	return 0;
}

static int netdag_gen_afspec_add(struct dagger *net, struct lyd_node *dif,
				 struct lyd_node *cif, FILE *ip)
{
	const char *iftype = lydx_get_cattr(cif, "type");
	int err = 0;

	DEBUG_IFACE(dif, "");

	if (!strcmp(iftype, "infix-if-type:bridge")) {
		err = netdag_gen_bridge(net, dif, cif, ip, 1);
	} else if (!strcmp(iftype, "infix-if-type:veth")) {
		err = netdag_gen_veth(net, NULL, cif, ip);
	} else if (!strcmp(iftype, "infix-if-type:vlan")) {
		err = netdag_gen_vlan(net, NULL, cif, ip);
	} else {
		ERROR("unsupported interface type \"%s\"", iftype);
		return -ENOSYS;
	}

	if (err)
		return err;

	return 0;
}

static int netdag_gen_afspec_set(struct dagger *net, struct lyd_node *dif,
				 struct lyd_node *cif, FILE *ip)
{
	const char *iftype = lydx_get_cattr(cif, "type");

	DEBUG_IFACE(dif, "");

	if (!strcmp(iftype, "infix-if-type:bridge"))
		return netdag_gen_bridge(net, dif, cif, ip, 0);
	if (!strcmp(iftype, "infix-if-type:vlan"))
		return netdag_gen_vlan(net, dif, cif, ip);
	if (!strcmp(iftype, "infix-if-type:veth"))
		return 0;

	ERROR("unsupported interface type \"%s\"", iftype);
	return -ENOSYS;
}

static bool netdag_must_del(struct lyd_node *dif, struct lyd_node *cif)
{
	const char *iftype = lydx_get_cattr(cif, "type");

	if (!strcmp(iftype, "infix-if-type:bridge"))
		return 0;
	if (!strcmp(iftype, "infix-if-type:vlan"))
		return lydx_get_cattr(dif, "parent-interface") ||
			lydx_get_descendant(lyd_child(dif),
					    "encapsulation",
					    "dot1q-vlan",
					    "outer-tag",
					    NULL);
	if (!strcmp(iftype, "infix-if-type:veth"))
		return lydx_get_descendant(lyd_child(dif), "peer", NULL);

	return false;
}

static int netdag_gen_iface_del(struct dagger *net, struct lyd_node *dif,
				       struct lyd_node *cif, bool fixed)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	FILE *ip;

	DEBUG_IFACE(dif, "");

	if (dagger_should_skip_current(net, ifname))
		return 0;

	ip = dagger_fopen_current(net, "exit", ifname, 50, "exit.ip");
	if (!ip)
		return -EIO;

	if (fixed) {
		fprintf(ip, "link set dev %s down\n", ifname);
		fprintf(ip, "addr flush dev %s\n", ifname);
	} else {
		fprintf(ip, "link del dev %s\n", ifname);
	}

	fclose(ip);
	return 0;
}

static sr_error_t netdag_gen_iface(struct dagger *net,
				   struct lyd_node *dif, struct lyd_node *cif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	enum lydx_op op = lydx_get_op(dif);
	const char *attr;
	int err = 0;
	bool fixed;
	FILE *ip;

	fixed = iface_is_phys(ifname) || !strcmp(ifname, "lo");

	DEBUG("%s(%s) %s", ifname, fixed ? "fixed" : "dynamic",
	      (op == LYDX_OP_NONE) ? "mod" : ((op == LYDX_OP_CREATE) ? "add" : "del"));

	if (op == LYDX_OP_DELETE) {
		err = netdag_gen_iface_del(net, dif, cif, fixed);
		goto err;
	}

	/* Although, from a NETCONF perspective, we are handling a
	 * modification, we may have to remove the interface and
	 * recreate it from scratch.  E.g. Linux can't modify the
	 * parent ("link") of an existing interface, but this is
	 * perfectly legal according to the YANG model.
	 */
	if (op != LYDX_OP_CREATE && netdag_must_del(dif, cif)) {
		DEBUG_IFACE(dif, "Must delete");

		err = netdag_gen_iface_del(net, dif, cif, fixed);
		if (err)
			goto err;

		/* Interface has been removed, convert the config to a
		 * diff, so that all settings/addresses are applied
		 * again.
		 */
		lyd_new_meta(cif->schema->module->ctx, cif, NULL,
			     "yang:operation", "create", false, NULL);
		dif = cif;
		op = LYDX_OP_CREATE;
	}

	ip = dagger_fopen_next(net, "init", ifname, 50, "init.ip");
	if (!ip) {
		err = -EIO;
		goto err;
	}

	if (!fixed && op == LYDX_OP_CREATE) {
		err = netdag_gen_afspec_add(net, dif, cif, ip);
		if (err)
			goto err_close_ip;
	}

	fprintf(ip, "link set dev %s down", ifname);

	/* Set generic link attributes */
	err = err ? : netdag_gen_ipv4_autoconf(net, cif, dif);
	err = err ? : netdag_gen_ipv6_autoconf(net, cif, dif, ip);
	if (err)
		goto err_close_ip;

	fputc('\n', ip);

	err = bridge_gen_ports(net, dif, cif, ip);
	if (err)
		goto err_close_ip;

	/* Set type specific attributes */
	if (!fixed && op != LYDX_OP_CREATE) {
		err = netdag_gen_afspec_set(net, dif, cif, ip);
		if (err)
			goto err_close_ip;
	}

	/* Set Addresses */
	err = err ? : netdag_gen_link_addr(ip, cif, dif);
	err = err ? : netdag_gen_ip_addrs(ip, "ipv4", cif, dif);
	err = err ? : netdag_gen_ip_addrs(ip, "ipv6", cif, dif);
	if (err)
		goto err_close_ip;

	/* Bring interface back up, if enabled */
	attr = lydx_get_cattr(cif, "enabled");
	if (!attr || !strcmp(attr, "true"))
		fprintf(ip, "link set dev %s up state up\n", ifname);

	err = err ? : netdag_gen_sysctl(net, dif);

err_close_ip:
	fclose(ip);
err:
	if (err)
		ERROR("Failed setting up %s: %d", ifname, err);

	return err ? SR_ERR_INTERNAL : SR_ERR_OK;
}

static sr_error_t netdag_init(sr_session_ctx_t *session, struct dagger *net,
			      struct lyd_node *cifs, struct lyd_node *difs)
{
	struct lyd_node *iface;

	LYX_LIST_FOR_EACH(cifs, iface, "interface")
		if (dagger_add_node(net, lydx_get_cattr(iface, "name")))
			return SR_ERR_INTERNAL;

	LYX_LIST_FOR_EACH(difs, iface, "interface")
		if (dagger_add_node(net, lydx_get_cattr(iface, "name")))
			return SR_ERR_INTERNAL;

	net->session = session;
	return SR_ERR_OK;
}

static int ifchange(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *diff, *cifs, *difs, *cif, *dif;
	struct confd *confd = _confd;
	sr_data_t *cfg;
	sr_error_t err;

	switch (event) {
	case SR_EV_CHANGE:
		break;
	case SR_EV_ABORT:
		return dagger_abandon(&confd->netdag);
	case SR_EV_DONE:
		return dagger_evolve_or_abandon(&confd->netdag);
	default:
		return SR_ERR_OK;
	}

	err = dagger_claim(&confd->netdag, "/run/net");
	if (err)
		return err;

	err = sr_get_data(session, "/interfaces/interface", 0, 0, 0, &cfg);
	if (err)
		goto err_abandon;

	err = srx_get_diff(session, (struct lyd_node **)&diff);
	if (err)
		goto err_release_data;

	cifs = lydx_get_descendant(cfg->tree, "interfaces", "interface", NULL);
	difs = lydx_get_descendant(diff, "interfaces", "interface", NULL);

	err = netdag_init(session, &confd->netdag, cifs, difs);
	if (err)
		goto err_free_diff;

	LYX_LIST_FOR_EACH(difs, dif, "interface") {
		LYX_LIST_FOR_EACH(cifs, cif, "interface")
			if (!strcmp(lydx_get_cattr(dif, "name"),
				    lydx_get_cattr(cif, "name")))
				break;

		err = netdag_gen_iface(&confd->netdag, dif, cif);
		if (err)
			break;
	}

err_free_diff:
	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:
	if (err)
		dagger_abandon(&confd->netdag);

	return err;
}

int ietf_interfaces_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_CHANGE(confd->session, "ietf-interfaces", "/ietf-interfaces:interfaces//.",
			0, ifchange, confd, &confd->sub);
	REGISTER_CHANGE(confd->cand, "ietf-interfaces", "/ietf-interfaces:interfaces//.",
			SR_SUBSCR_UPDATE, ifchange_cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("failed, error %d: %s", rc, sr_strerror(rc));
	return rc;
}
