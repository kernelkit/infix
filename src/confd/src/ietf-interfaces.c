/* SPDX-License-Identifier: BSD-3-Clause */

#include <fnmatch.h>
#include <stdbool.h>
#include <jansson.h>
#include <arpa/inet.h>
#include <net/if.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "ietf-interfaces.h"

static bool iface_is_phys(const char *ifname)
{
	bool is_phys = false;
	json_error_t jerr;
	const char *attr;
	json_t *link;
	FILE *proc;

	proc = cni_popen("ip -d -j link show dev %s 2>/dev/null", ifname);
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

static int ifchange_cand_infer_type(sr_session_ctx_t *session, const char *path)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	char *ifname, *type, *xpath;
	sr_error_t err = SR_ERR_OK;

	xpath = xpath_base(path);
	if (!xpath)
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
	else if (!fnmatch("bond+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:lag";
	else if (!fnmatch("lag+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:lag";
	else if (!fnmatch("docker+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:bridge";
	else if (!fnmatch("dummy+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:dummy";
	else if (!fnmatch("podman+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:bridge";
	else if (!fnmatch("lag+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:lag";
	else if (!fnmatch("veth+([0-9a-z_-])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:veth";
	else if (!fnmatch("vlan+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:vlan";
	else if (!fnmatch("*.+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:vlan";
	else if (!fnmatch("gre+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:gre";
	else if (!fnmatch("gretap+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:gretap";
	else if (!fnmatch("vxlan+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "infix-if-type:vxlan";
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

		err = cni_ifchange_cand_infer_type(session, new->xpath);
		if (err)
			break;
	}

	sr_free_change_iter(iter);
	return SR_ERR_OK;
}

static int netdag_gen_link_mtu(FILE *ip, struct lyd_node *dif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	struct lyd_node *node;
	struct lydx_diff nd;

	if (!strcmp(ifname, "lo")) /* skip for now */
		return 0;

	node = lydx_get_descendant(lyd_child(dif), "ipv4", "mtu", NULL);
	if (node && lydx_get_diff(node, &nd))
		fprintf(ip, "link set %s mtu %s\n", ifname, nd.new ? nd.val : "1500");

	return 0;
}

static void calc_mac(const char *base_mac, const char *mac_offset, char *buf)
{
	uint8_t base[6], offset[6], result[6];
	int carry = 0, i;

	sscanf(base_mac, "%hhx:%hhx:%hhx:%hhx:%hhx:%hhx",
	       &base[0], &base[1], &base[2], &base[3], &base[4], &base[5]);

	sscanf(mac_offset, "%hhx:%hhx:%hhx:%hhx:%hhx:%hhx",
	       &offset[0], &offset[1], &offset[2], &offset[3], &offset[4], &offset[5]);

	for (i = 5; i >= 0; i--) {
		int sum = base[i] + offset[i] + carry;

		result[i] = sum & 0xFF;
		carry = (sum > 0xFF) ? 1 : 0;
	}

	sprintf(buf, "%02x:%02x:%02x:%02x:%02x:%02x",
		result[0], result[1], result[2], result[3], result[4], result[5]);
}

const char *get_chassis_addr(void)
{
	struct json_t *addr;

	addr = json_object_get(confd.root, "mac-address");
	if (!addr) {
		WARN("No chassis MAC found.");
		return NULL;
	}

	return json_string_value(addr);
}

static int get_phys_addr(struct lyd_node *cif, char *mac)
{
	struct lyd_node *cpa, *chassis;
	const char *base, *offs;

	cpa = lydx_get_child(cif, "custom-phys-address");
	if (!cpa)
		return -ENODATA;

	base = lydx_get_cattr(cpa, "static");
	if (base) {
		strcpy(mac, base);
		return 0;
	}

	chassis = lydx_get_child(cpa, "chassis");
	if (chassis) {
		base = get_chassis_addr() ? : "00:00:00:00:00:00";
		offs = lydx_get_cattr(chassis, "offset") ? : "00:00:00:00:00:00";
		calc_mac(base, offs, mac);
		return 0;
	}

	return -ENODATA;
}

int link_gen_address(struct lyd_node *cif, FILE *ip)
{
	char mac[18];
	int err;

	err = get_phys_addr(cif, mac);
	if (err)
		return err;

	fprintf(ip, " address %s", mac);
	return 0;
}

static int netdag_gen_link_addr(FILE *ip, struct lyd_node *cif, struct lyd_node *dif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	char mac[18];
	int err;

	if (!lydx_get_child(dif, "custom-phys-address"))
		return 0;

	err = get_phys_addr(cif, mac);
	if (err)
		return (err == -ENODATA) ? 0 : err;

	fprintf(ip, "link set %s address %s\n", ifname, mac);
	return 0;
}

static int netdag_gen_sysctl_setting(struct dagger *net, const char *ifname, FILE **fpp,
				     int isboolean, const char *fallback,
				     struct lyd_node *node, const char *fmt, ...)
{
	struct lydx_diff nd;
	const char *value;
	va_list ap;

	if (!node)
		return 0;

	if (!lydx_get_diff(node, &nd))
		return 0;

	*fpp = *fpp ? : dagger_fopen_net_init(net, ifname,
					      NETDAG_INIT_POST, "init.sysctl");
	if (!*fpp)
		return -EIO;

	va_start(ap, fmt);
	vfprintf(*fpp, fmt, ap);
	va_end(ap);

	if (isboolean) {
		if (nd.new && !strcmp(nd.val, "true"))
			value = "1";
		else
			value = "0";
	} else {
		if (nd.new)
			value = nd.val;
		else
			value = fallback;
	}

	fprintf(*fpp, " = %s\n", value);

	return 0;
}

static int netdag_gen_sysctl(struct dagger *net,
			     struct lyd_node *cif,
			     struct lyd_node *dif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	struct lyd_node *node;
	FILE *sysctl = NULL;
	int err = 0;

	node = lydx_get_descendant(lyd_child(dif), "ipv4", "forwarding", NULL);
	err = err ? : netdag_gen_sysctl_setting(net, ifname, &sysctl, 1, "0", node,
						"net.ipv4.conf.%s.forwarding", ifname);

	node = lydx_get_descendant(lyd_child(dif), "ipv6", "forwarding", NULL);
	err = err ? : netdag_gen_sysctl_setting(net, ifname, &sysctl, 1, "0", node,
						"net.ipv6.conf.%s.forwarding", ifname);

	if (!strcmp(ifname, "lo")) /* skip for now */
		goto skip_mtu;

	node = lydx_get_descendant(lyd_child(cif), "ipv6", "mtu", NULL);
	err = err ? : netdag_gen_sysctl_setting(net, ifname, &sysctl, 0, "1280", node,
						"net.ipv6.conf.%s.mtu", ifname);

skip_mtu:
	if (sysctl)
		fclose(sysctl);

	return err;
}

static int dummy_gen(struct lyd_node *dif, struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");

	fprintf(ip, "link add dev %s", ifname);
	link_gen_address(cif, ip);
	fputs(" type dummy\n", ip);
	return 0;
}

static int netdag_gen_afspec_add(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *dif,
				 struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");

	DEBUG_IFACE(dif, "");

	switch (iftype_from_iface(cif)) {
	case IFT_BRIDGE:
		return bridge_gen(dif, cif, ip, 1);
	case IFT_DUMMY:
		return dummy_gen(NULL, cif, ip);
	case IFT_GRE:
	case IFT_GRETAP:
		return gre_gen(NULL, cif, ip);
	case IFT_LAG:
		return lag_gen(dif, cif, ip, 1);
	case IFT_VETH:
		return veth_gen(NULL, cif, ip);
	case IFT_VLAN:
		return vlan_gen(NULL, cif, ip);
	case IFT_VXLAN:
		return vxlan_gen(NULL, cif, ip);

	case IFT_ETH:
	case IFT_ETHISH:
	case IFT_LO:
	case IFT_UNKNOWN:
		sr_session_set_error_message(net->session, "%s: unsupported interface type \"%s\"",
					     ifname, lydx_get_cattr(cif, "type"));
	}

	__builtin_unreachable();
	return -EINVAL;
}

static int netdag_gen_afspec_set(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *dif,
				 struct lyd_node *cif, FILE *ip)
{
	DEBUG_IFACE(dif, "");

	switch (iftype_from_iface(cif)) {
	case IFT_BRIDGE:
		return bridge_gen(dif, cif, ip, 0);
	case IFT_LAG:
		return lag_gen(dif, cif, ip, 0);
	case IFT_VLAN:
		return vlan_gen(dif, cif, ip);

	case IFT_DUMMY:
	case IFT_GRE:
	case IFT_GRETAP:
	case IFT_VETH:
	case IFT_VXLAN:
		return 0;

	case IFT_ETH:
	case IFT_ETHISH:
	case IFT_LO:
	case IFT_UNKNOWN:
		return ERR_IFACE(cif, -ENOSYS, "unsupported interface type \"%s\"",
				 lydx_get_cattr(cif, "type"));
	}

	__builtin_unreachable();
	return -EINVAL;
}

static bool netdag_must_del(struct lyd_node *dif, struct lyd_node *cif)
{
	switch (iftype_from_iface(cif)) {
	case IFT_BRIDGE:
	case IFT_DUMMY:
	case IFT_LO:
		break;
	case IFT_ETH:
	case IFT_ETHISH:
	/* case IFT_LAG: */
	/* 	... REMEMBER WHEN ADDING BOND SUPPORT ... */
		return lydx_get_child(dif, "custom-phys-address");
	case IFT_GRE:
	case IFT_GRETAP:
		return lydx_get_descendant(lyd_child(dif), "gre", NULL);
	case IFT_LAG:
		return lydx_get_child(dif, "custom-phys-address") ||
			lydx_get_descendant(lyd_child(dif), "lag", "mode", NULL);
	case IFT_VLAN:
		return lydx_get_descendant(lyd_child(dif), "vlan", NULL);
	case IFT_VETH:
		return lydx_get_descendant(lyd_child(dif), "veth", NULL);
	case IFT_VXLAN:
		return lydx_get_descendant(lyd_child(dif), "vxlan", NULL);
	case IFT_UNKNOWN:
		ERR_IFACE(cif, -EINVAL, "unsupported interface type \"%s\"",
			  lydx_get_cattr(cif, "type"));
		return true;
	}

	return false;
}

static int eth_gen_del(struct lyd_node *dif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	char mac[18];
	FILE *pp;

	fprintf(ip, "link set dev %s down", ifname);

	pp = cni_popen("ip -d -j link show dev %s | jq -rM .[].permaddr", ifname);
	if (pp) {
		if (fgets(mac, sizeof(mac), pp) && !strstr(mac, "null"))
			fprintf(ip, " address %s", mac);
		pclose(pp);
	}

	fputc('\n', ip);

	fprintf(ip, "addr flush dev %s\n", ifname);
	return 0;
}

static int link_gen_del(struct lyd_node *dif, FILE *ip)
{
	fprintf(ip, "link del dev %s\n", lydx_get_cattr(dif, "name"));
	return 0;
}

static int veth_gen_del(struct lyd_node *dif, FILE *ip)
{
	if (!veth_is_primary(dif))
		return 0;

	return link_gen_del(dif, ip);
}

static int netdag_gen_iface_del(struct dagger *net, struct lyd_node *dif,
				struct lyd_node *cif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	enum iftype type;
	FILE *ip;

	DEBUG_IFACE(dif, "");

	ip = dagger_fopen_net_exit(net, ifname, NETDAG_EXIT, "exit.ip");
	if (!ip)
		return -EIO;

	type = iftype_from_iface(dif);
	if (type == IFT_UNKNOWN)
		/* The interface is still in running, so we need to
		 * get the type from there. This can happen when an
		 * attribute is changed that require us to delete and
		 * recreate the interface, e.g. changing the VID of a
		 * VLAN interface. */
		type = iftype_from_iface(cif);

	switch (type) {
	case IFT_ETH:
	case IFT_ETHISH:
	case IFT_LO:
		eth_gen_del(dif, ip);
		break;
	case IFT_VETH:
		veth_gen_del(dif, ip);
		break;
	case IFT_BRIDGE:
	case IFT_DUMMY:
	case IFT_GRE:
	case IFT_GRETAP:
	case IFT_LAG:
	case IFT_VLAN:
	case IFT_VXLAN:
	case IFT_UNKNOWN:
		link_gen_del(dif, ip);
		break;
	}

	fclose(ip);
	return 0;
}

static sr_error_t netdag_gen_iface(sr_session_ctx_t *session, struct dagger *net,
				   struct lyd_node *dif, struct lyd_node *cif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	enum lydx_op op = lydx_get_op(dif);
	const char *attr;
	int err = 0;
	bool fixed;
	FILE *ip;

	if ((err = cni_netdag_gen_iface(net, ifname, dif, cif))) {
		/* error or managed by CNI/podman */
		if (err > 0)
			err = 0; /* done, nothing more to do here */
		goto err;
	}

	fixed = iface_is_phys(ifname) || !strcmp(ifname, "lo");

	DEBUG("%s(%s) %s", ifname, fixed ? "fixed" : "dynamic",
	      (op == LYDX_OP_NONE) ? "mod" : ((op == LYDX_OP_CREATE) ? "add" : "del"));

	if (op == LYDX_OP_DELETE) {
		err  = netdag_gen_iface_del(net, dif, cif);
		err += netdag_gen_ipv4_autoconf(net, cif, dif);
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

		err = netdag_gen_iface_del(net, dif, cif);
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

	ip = dagger_fopen_net_init(net, ifname, NETDAG_INIT, "init.ip");
	if (!ip) {
		err = -EIO;
		goto err;
	}

	if (!fixed && op == LYDX_OP_CREATE) {
		err = netdag_gen_afspec_add(session, net, dif, cif, ip);
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

	err = bridge_port_gen(dif, cif, ip);
	if (err)
		goto err_close_ip;

	err = lag_port_gen(dif, cif);
	if (err)
		goto err_close_ip;

	/* Set type specific attributes */
	if (!fixed && op != LYDX_OP_CREATE) {
		err = netdag_gen_afspec_set(session, net, dif, cif, ip);
		if (err)
			goto err_close_ip;
	}

	/* Set Addresses */
	err = err ? : netdag_gen_link_mtu(ip, dif);
	err = err ? : netdag_gen_link_addr(ip, cif, dif);
	err = err ? : netdag_gen_ip_addrs(net, ip, "ipv4", cif, dif);
	err = err ? : netdag_gen_ip_addrs(net, ip, "ipv6", cif, dif);
	if (err)
		goto err_close_ip;

	/* ifAlias, should skip for container-network types */
	attr = lydx_get_cattr(cif, "description");
	fprintf(ip, "link set alias \"%s\" dev %s\n", attr ?: "", ifname);

	/* Bring interface back up, if enabled */
	if (lydx_is_enabled(cif, "enabled"))
		fprintf(ip, "link set dev %s up state up\n", ifname);

	err = err ? : netdag_gen_sysctl(net, cif, dif);

	err = err ? : netdag_gen_ethtool(net, cif, dif);

err_close_ip:
	fclose(ip);
err:
	if (err)
		ERROR("Failed setting up %s: %d", ifname, err);

	return err ? SR_ERR_INTERNAL : SR_ERR_OK;
}

static int netdag_init_iface(struct lyd_node *cif)
{
	int err;

	err = dagger_add_node(&confd.netdag, lydx_get_cattr(cif, "name"));
	if (err)
		return err;

	switch (iftype_from_iface(cif)) {
	case IFT_BRIDGE:
		return bridge_add_deps(cif);
	case IFT_LAG:
		return lag_add_deps(cif);
	case IFT_VLAN:
		return vlan_add_deps(cif);
	case IFT_VETH:
		return veth_add_deps(cif);

	case IFT_DUMMY:
	case IFT_ETH:
	case IFT_ETHISH:
	case IFT_GRE:
	case IFT_GRETAP:
	case IFT_LO:
	case IFT_VXLAN:
	case IFT_UNKNOWN:
		break;
	}

	return 0;
}

static sr_error_t netdag_init(sr_session_ctx_t *session, struct dagger *net,
			      struct lyd_node *cifs, struct lyd_node *difs)
{
	struct lyd_node *cif;
	int err;

	LYX_LIST_FOR_EACH(cifs, cif, "interface") {
		err = netdag_init_iface(cif);
		if (err)
			return SR_ERR_INTERNAL;
	}

	net->session = session;
	return SR_ERR_OK;
}

static sr_error_t ifchange_post(sr_session_ctx_t *session, struct dagger *net,
				struct lyd_node *cifs, struct lyd_node *difs)
{
	int err;

	/* For each configured bridge, the corresponding multicast
	 * querier settings depend on both the bridge config and on
	 * the presence of matching VLAN uppers.  Since these can be
	 * independently configured - the upper might exist in difs
	 * but not the bridge, or vice versa - it is much easier to
	 * regenerate the full config for mcd every time by walking
	 * the full configuration.
	 */
	err = bridge_mcd_gen(cifs);

	/* Whenever at least one bridge has spanning tree enabled,
	 * start mstpd; otherwise, stop it.
	 */
	err = err ? : bridge_mstpd_gen(cifs);

	return err ? SR_ERR_INTERNAL : SR_ERR_OK;
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
		if (!dagger_evolve_or_abandon(&confd->netdag))
			return SR_ERR_OK;

		ERROR("Failed to apply interface configuration");
		return SR_ERR_INTERNAL;
	default:
		return SR_ERR_OK;
	}

	err = dagger_claim(&confd->netdag, "/run/net");
	if (err)
		return err;

	err = sr_get_data(session, "/interfaces/interface", 0, 0, 0, &cfg);
	if (err || !cfg)
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

		err = netdag_gen_iface(session, &confd->netdag, dif, cif);
		if (err)
			break;
	}

	err = err ? : ifchange_post(session, &confd->netdag, cifs, difs);

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
	int rc;

	REGISTER_CHANGE(confd->session, "ietf-interfaces", "/ietf-interfaces:interfaces//.",
			0, ifchange, confd, &confd->sub);
	REGISTER_CHANGE(confd->cand, "ietf-interfaces", "/ietf-interfaces:interfaces//.",
			SR_SUBSCR_UPDATE, ifchange_cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("failed, error %d: %s", rc, sr_strerror(rc));
	return rc;
}
