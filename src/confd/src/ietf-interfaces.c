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

static void calc_mac(const char *base_mac, const char *mac_offset, char *buf, size_t len)
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

	snprintf(buf, len, "%02x:%02x:%02x:%02x:%02x:%02x",
		 result[0], result[1], result[2], result[3], result[4], result[5]);
}

/*
 * Get child value from a diff parent, only returns value if not
 * deleted.  In which case the deleted flag may be set.
 */
static const char *get_val(struct lyd_node *parent, char *name, int *deleted)
{
	const char *value = NULL;
	struct lyd_node *node;

	node = lydx_get_child(parent, name);
	if (node) {
		if (lydx_get_op(node) == LYDX_OP_DELETE) {
			if (deleted)
				*deleted = 1;
			return NULL;
		}

		value = lyd_get_value(node);
	}

	return value;
}

/*
 * Locate custom-phys-address, adjust for any offset, and return pointer
 * to a static string.  (Which will be overwritten on subsequent calls.)
 *
 * The 'deleted' flag will be set if any of the nodes in the subtree are
 * deleted.  Used when restoring permaddr and similar.
 */
char *get_phys_addr(struct lyd_node *parent, int *deleted)
{
	struct lyd_node *node, *cpa;
	static char mac[18];
	struct json_t *j;
	const char *ptr;

	cpa = lydx_get_descendant(lyd_child(parent), "custom-phys-address", NULL);
	if (!cpa || lydx_get_op(cpa) == LYDX_OP_DELETE) {
		if (cpa && deleted)
			*deleted = 1;
		return NULL;
	}

	ptr = get_val(cpa, "static", deleted);
	if (ptr) {
		strlcpy(mac, ptr, sizeof(mac));
		return mac;
	}

	node = lydx_get_child(cpa, "chassis");
	if (!node || lydx_get_op(node) == LYDX_OP_DELETE) {
		if (node && deleted)
			*deleted = 1;
		return NULL;
	}

	j = json_object_get(confd.root, "mac-address");
	if (!j) {
		WARN("cannot set chassis based MAC, not found.");
		return NULL;
	}

	ptr = json_string_value(j);
	strlcpy(mac, ptr, sizeof(mac));

	ptr = get_val(node, "offset", deleted);
	if (ptr)
		calc_mac(mac, ptr, mac, sizeof(mac));

	return mac;
}

static int netdag_gen_link_addr(FILE *ip, struct lyd_node *cif, struct lyd_node *dif)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	const char *mac;
	int deleted = 0;
	char buf[32];

	mac = get_phys_addr(dif, &deleted);
	if (!mac && deleted) {
		FILE *fp;

		/*
		 * Only physical interfaces support this, virtual ones
		 * we remove, see netdag_must_del() for details.
		 */
		fp = cni_popen("ip -d -j link show dev %s |jq -rM .[].permaddr", ifname);
		if (fp) {
			if (fgets(buf, sizeof(buf), fp))
				mac = chomp(buf);
			pclose(fp);

			if (mac && !strcmp(mac, "null"))
				return 0;
		}
	}

	if (!mac || !strlen(mac)) {
		DEBUG("No change in %s phys-address, skipping ...", ifname);
		return 0;
	}

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

static int netdag_gen_dummy(struct dagger *net, struct lyd_node *dif,
			    struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");

	fprintf(ip, "link add dev %s type dummy\n", ifname);

	return 0;
}

static int netdag_gen_afspec_add(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *dif,
				 struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	const char *iftype = lydx_get_cattr(cif, "type");
	int err = 0;

	DEBUG_IFACE(dif, "");

	if (!strcmp(iftype, "infix-if-type:bridge")) {
		err = bridge_gen(dif, cif, ip, 1);
	} else if (!strcmp(iftype, "infix-if-type:dummy")) {
		err = netdag_gen_dummy(net, NULL, cif, ip);
	} else if (!strcmp(iftype, "infix-if-type:veth")) {
		err = netdag_gen_veth(net, NULL, cif, ip);
	} else if (!strcmp(iftype, "infix-if-type:vlan")) {
		err = netdag_gen_vlan(net, NULL, cif, ip);
	} else if (!strcmp(iftype, "infix-if-type:ethernet")) {
		sr_session_set_error_message(net->session, "Cannot create fixed Ethernet interface %s,"
					     " wrong type or name.", ifname);
		return -ENOENT;
	} else if (!strcmp(iftype, "infix-if-type:gre") || !strcmp(iftype, "infix-if-type:gretap")) {
		err = gre_gen(net, NULL, cif, ip);
	} else if (!strcmp(iftype, "infix-if-type:vxlan")) {
		err = vxlan_gen(net, NULL, cif, ip);
	} else {
		sr_session_set_error_message(net->session, "%s: unsupported interface type \"%s\"", ifname, iftype);
		return -ENOSYS;
	}

	if (err)
		return err;

	return 0;
}

static int netdag_gen_afspec_set(sr_session_ctx_t *session, struct dagger *net, struct lyd_node *dif,
				 struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	const char *iftype = lydx_get_cattr(cif, "type");

	DEBUG_IFACE(dif, "");

	if (!strcmp(iftype, "infix-if-type:bridge"))
		return bridge_gen(dif, cif, ip, 0);
	if (!strcmp(iftype, "infix-if-type:vlan"))
		return netdag_gen_vlan(net, dif, cif, ip);
	if (!strcmp(iftype, "infix-if-type:veth"))
		return 0;
	if (!strcmp(iftype, "infix-if-type:gretap"))
		return 0;
	if (!strcmp(iftype, "infix-if-type:vxlan"))
		return 0;
	ERROR("%s: unsupported interface type \"%s\"", ifname, iftype);
	return -ENOSYS;
}

static bool is_phys_addr_deleted(struct lyd_node *dif)
{
	int deleted = 0;

	if (!get_phys_addr(dif, &deleted) && deleted)
		return true;

	return false;
}

static bool netdag_must_del(struct lyd_node *dif, struct lyd_node *cif)
{
	const char *iftype = lydx_get_cattr(cif, "type");

	if (strcmp(iftype, "infix-if-type:ethernet") &&
	    strcmp(iftype, "infix-if-type:etherlike")) {
		if (is_phys_addr_deleted(dif))
			return true;
	}

	if (!strcmp(iftype, "infix-if-type:vlan")) {
		if (lydx_get_descendant(lyd_child(dif), "vlan", NULL))
			return true;
	} else if (!strcmp(iftype, "infix-if-type:veth")) {
		if (lydx_get_descendant(lyd_child(dif), "veth", NULL))
			return true;
	} else if (!strcmp(iftype, "infix-if-type:gre") || !strcmp(iftype, "infix-if-type:gretap")) {
		if (lydx_get_descendant(lyd_child(dif), "gre", NULL))
			return true;
	} else if (!strcmp(iftype, "infix-if-type:vxlan")) {
		if (lydx_get_descendant(lyd_child(dif), "vxlan", NULL))
			return true;
/*
	} else if (!strcmp(iftype, "infix-if-type:lag")) {
		if (is_phys_addr_deleted(dif))
			return true;

		... REMEMBER WHEN ADDING BOND SUPPORT ...
*/
	}

	return false;
}

static int netdag_gen_iface_del(struct dagger *net, struct lyd_node *dif,
				       struct lyd_node *cif, bool fixed)
{
	const char *ifname = lydx_get_cattr(dif, "name");
	const char *iftype = lydx_get_cattr(dif, "type");
	FILE *ip;

	DEBUG_IFACE(dif, "");

	if (dagger_should_skip_current(net, ifname))
		return 0;

	if (iftype && !strcmp(iftype, "infix-if-type:veth")) {
		struct lyd_node *node;
		const char *peer;

		node = lydx_get_descendant(lyd_child(dif), "veth", NULL);
		if (!node)
			return -EINVAL;

		peer = lydx_get_cattr(node, "peer");
		if (!peer)
			return -EINVAL;

		dagger_skip_current_iface(net, peer);
	}

	ip = dagger_fopen_net_exit(net, ifname, NETDAG_EXIT, "exit.ip");
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
		err  = netdag_gen_iface_del(net, dif, cif, fixed);
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
	attr = lydx_get_cattr(cif, "enabled");
	if (!attr || !strcmp(attr, "true"))
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

static sr_error_t netdag_init(sr_session_ctx_t *session, struct dagger *net,
			      struct lyd_node *cifs, struct lyd_node *difs)
{
	struct lyd_node *iface;

	LYX_LIST_FOR_EACH(cifs, iface, "interface")
		if (dagger_add_node(net, lydx_get_cattr(iface, "name")))
			return SR_ERR_INTERNAL;

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
