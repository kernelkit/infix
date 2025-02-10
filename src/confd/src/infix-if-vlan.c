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

int ifchange_cand_infer_vlan(sr_session_ctx_t *session, const char *path)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	char *ifname, *type, *xpath, *lower;
	sr_error_t err = SR_ERR_OK;
	size_t cnt = 0;
	int vid;

	xpath = xpath_base(path);
	if (!xpath)
		return SR_ERR_SYS;

	type = srx_get_str(session, "%s/type", xpath);
	if (!type)
		goto out;

	if (strcmp(type, "infix-if-type:vlan"))
		goto out_free_type;

	ifname = srx_get_str(session, "%s/name", xpath);
	if (!ifname)
		goto out_free_type;

	if (!fnmatch("*.+([0-9])", ifname, FNM_EXTMATCH)) {
		char *ptr = rindex(ifname, '.');

		if (!ptr)
			goto out_free_ifname;

		*ptr++ = '\0';
		vid    = strtol(ptr, NULL, 10);
		lower  = ifname;
	} else if (!fnmatch("vlan+([0-9])", ifname, FNM_EXTMATCH)) {
		if (sscanf(ifname, "vlan%d", &vid) != 1)
		    goto out_free_ifname;

		/* Avoid setting lower-layer-if to vlanN */
		lower = NULL;
	} else {
		goto out_free_ifname;
	}

	if (vid < 1 || vid > 4094)
		goto out_free_ifname;

	if (lower) {
		err = srx_nitems(session, &cnt, "/interfaces/interface[name='%s']/name", lower);
		if (err || !cnt)
			goto out_free_ifname;

		err = srx_nitems(session, &cnt, IF_VLAN_XPATH "/lower-layer-if", xpath);
		if (!err && !cnt) {
			inferred.data.string_val = lower;
			err = srx_set_item(session, &inferred, 0, IF_VLAN_XPATH "/lower-layer-if", xpath);
			if (err)
				goto out_free_ifname;
		}
	}

	err = srx_nitems(session, &cnt, IF_VLAN_XPATH "/tag-type", xpath);
	if (!err && !cnt) {
		inferred.data.string_val = "ieee802-dot1q-types:c-vlan";
		err = srx_set_item(session, &inferred, 0, IF_VLAN_XPATH "/tag-type", xpath);
		if (err)
			goto out_free_ifname;
	}

	err = srx_nitems(session, &cnt, IF_VLAN_XPATH "/id", xpath);
	if (!err && !cnt) {
		inferred.type = SR_INT32_T;
		inferred.data.int32_val = vid;
		err = srx_set_item(session, &inferred, 0, IF_VLAN_XPATH "/id", xpath);
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

static int netdag_gen_vlan_ingress_qos(struct lyd_node *cif, FILE *ip)
{
	const char *prio;

	prio = lyd_get_value(lydx_get_descendant(lyd_child(cif),
						 "vlan", "ingress-qos", "priority", NULL));

	if (prio[0] >= '0' && prio[0] <= '7' && prio[1] == '\0') {
		fprintf(ip, " ingress-qos-map 0:%c 1:%c 2:%c 3:%c 4:%c 5:%c 6:%c 7:%c",
			prio[0], prio[0], prio[0], prio[0], prio[0], prio[0], prio[0], prio[0]);
		return 0;
	} else if (!strcmp(prio, "from-pcp")) {
		fputs(" ingress-qos-map 0:0 1:1 2:2 3:3 4:4 5:5 6:6 7:7", ip);
		return 0;
	}

	return ERR_IFACE(cif, -EINVAL, "Unsupported ingress priority mode \"%s\"", prio);
}

static int netdag_gen_vlan_egress_qos(struct lyd_node *cif, FILE *ip)
{
	const char *pcp;

	pcp = lyd_get_value(lydx_get_descendant(lyd_child(cif),
						"vlan", "egress-qos", "pcp", NULL));

	if (pcp[0] >= '0' && pcp[0] <= '7' && pcp[1] == '\0') {
		fprintf(ip, " egress-qos-map 0:%c 1:%c 2:%c 3:%c 4:%c 5:%c 6:%c 7:%c",
			pcp[0], pcp[0], pcp[0], pcp[0], pcp[0], pcp[0], pcp[0], pcp[0]);
		return 0;
	} else if (!strcmp(pcp, "from-priority")) {
		fputs(" egress-qos-map 0:0 1:1 2:2 3:3 4:4 5:5 6:6 7:7", ip);
		return 0;
	}

	return ERR_IFACE(cif, -EINVAL, "Unsupported egress priority mode \"%s\"", pcp);
}

int netdag_gen_vlan(struct dagger *net, struct lyd_node *dif,
		    struct lyd_node *cif, FILE *ip)
{
	const char *ifname = lydx_get_cattr(cif, "name");
	struct lydx_diff typed, vidd;
	struct lyd_node *vlan;
	const char *lower_if;
	const char *proto;
	int err;

	vlan = lydx_get_descendant(lyd_child(dif ? : cif), "vlan", NULL);
	if (!vlan) {
		/*
		 * Note: this is only an error if vlan subcontext is missing
		 * from cif, otherwise it just means the interface had a
		 * a change that was not related to the VLAN config.
		 */
		if (!dif)
			ERROR("%s: missing mandatory vlan", ifname);
		return 0;
	}

	lower_if = lydx_get_cattr(vlan, "lower-layer-if");
	DEBUG("ifname %s lower if %s\n", ifname, lower_if);

	fprintf(ip, "link add dev %s down link %s type vlan", ifname, lower_if);

	if (lydx_get_diff(lydx_get_child(vlan, "tag-type"), &typed)) {
		proto = bridge_tagtype2str(typed.new);
		if (!proto)
			return ERR_IFACE(cif, -ENOSYS, "Unsupported tag type \"%s\"", typed.new);

		fprintf(ip, " proto %s", proto);
	}

	if (lydx_get_diff(lydx_get_child(vlan, "id"), &vidd))
		fprintf(ip, " id %s", vidd.new);

	err = netdag_gen_vlan_ingress_qos(cif, ip);
	if (err)
		return err;

	err = netdag_gen_vlan_egress_qos(cif, ip);
	if (err)
		return err;

	fputc('\n', ip);

	return 0;
}

int vlan_add_deps(struct lyd_node *cif)
{
	struct lyd_node *vlan = lydx_get_child(cif, "vlan");
	const char *lower;
	int err;

	lower = lydx_get_cattr(vlan, "lower-layer-if");

	err = dagger_add_dep(&confd.netdag, lydx_get_cattr(cif, "name"), lower);
	if (err)
		return ERR_IFACE(cif, err, "Unable to depend on \"%s\"", lower);

	return 0;
}
