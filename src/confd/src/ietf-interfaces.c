/* SPDX-License-Identifier: BSD-3-Clause */

#include <fnmatch.h>
#include <stdbool.h>

#include <jansson.h>

#include <net/if.h>

#include "core.h"
#include "lyx.h"
#include "netdo.h"
#include "srx_module.h"
#include "srx_val.h"

static const char *iffeat[] = {
	"if-mib",
	NULL
};

static const struct srx_module_requirement ietf_if_reqs[] = {
	{ .dir = YANG_PATH_, .name = "ietf-interfaces", .rev = "2018-02-20", .features = iffeat },
	{ .dir = YANG_PATH_, .name = "iana-if-type", .rev = "2023-01-26" },
	{ .dir = YANG_PATH_, .name = "ietf-ip", .rev = "2018-02-22" },
	{ .dir = YANG_PATH_, .name = "infix-ip", .rev = "2023-04-24" },

	{ NULL }
};

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

	if (!json_unpack(link, "[{s: { s:s }}]", "linkinfo", "info_kind", &attr))
		goto out_free;

	is_phys = true;

out_free:
	json_decref(link);
out:
	return is_phys;
}

static int ifchange_cand_infer_type(sr_session_ctx_t *session, const char *xpath)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	sr_error_t err = SR_ERR_OK;
	char *ifname, *type;

	type = srx_get_str(session, "%s/type", xpath);
	if (type) {
		free(type);
		return SR_ERR_OK;
	}

	ifname = srx_get_str(session, "%s/name", xpath);
	if (!ifname)
		return SR_ERR_INTERNAL;

	if (iface_is_phys(ifname))
		inferred.data.string_val = "iana-if-type:ethernetCsmacd";
	else if (!fnmatch("br+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "iana-if-type:bridge";
	else if (!fnmatch("lag+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "iana-if-type:ieee8023adLag";
	else if (!fnmatch("vlan+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "iana-if-type:l2vlan";
	else if (!fnmatch("*.+([0-9])", ifname, FNM_EXTMATCH))
		inferred.data.string_val = "iana-if-type:l2vlan";

	free(ifname);

	if (inferred.data.string_val)
		err = srx_set_item(session, &inferred, 0, "%s/type", xpath);

	return err;
}

static int ifchange_cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	sr_change_iter_t *iter;
	sr_change_oper_t op;
	sr_val_t *old, *new;
	sr_error_t err;

	if (event != SR_EV_UPDATE)
		return SR_ERR_OK;

	err = sr_dup_changes_iter(session, "/ietf-interfaces:interfaces/interface", &iter);
	if (err)
		return err;

	while (sr_get_change_next(session, iter, &op, &old, &new) == SR_ERR_OK) {
		if (op != SR_OP_CREATED)
			continue;

		err = ifchange_cand_infer_type(session, new->xpath);
		if (err)
			break;
	}

	sr_free_change_iter(iter);
	return SR_ERR_OK;
}

static sr_error_t ifchange_one(struct confd *confd, struct lyd_node *iface)
{
	const char *ifname = lydx_get_cattr(iface, "name");
	bool is_phys = iface_is_phys(ifname);
	enum lydx_op op = lydx_get_op(iface);
	const char *attr;
	FILE *ip;

	DEBUG("%s(%s) %s", ifname, is_phys ? "phys" : "virt",
	      (op == LYDX_OP_NONE) ? "mod" : ((op == LYDX_OP_CREATE) ? "add" : "del"));

	if (op == LYDX_OP_DELETE) {
		ip = netdo_get_file(&confd->netdo, ifname, NETDO_EXIT, NETDO_IP);
		if (!ip)
			return SR_ERR_INTERNAL;

		if (is_phys) {
			fprintf(ip, "link set dev %s down\n", ifname);
			fprintf(ip, "addr flush dev %s\n", ifname);
		} else {
			fprintf(ip, "link del dev %s\n", ifname);
		}

		return SR_ERR_OK;
	}

	ip = netdo_get_file(&confd->netdo, ifname, NETDO_INIT, NETDO_IP);
	if (!ip)
		return SR_ERR_INTERNAL;

	attr = ((op == LYDX_OP_CREATE) && !is_phys) ? "add" : "set";
	fprintf(ip, "link %s dev %s", attr, ifname);

	/* Generic attributes */

	attr = lydx_get_cattr(iface, "enabled");
	if (!attr && (op == LYDX_OP_CREATE))
		/* When adding an interface to the configuration, we
		 * need to bring it up, even when "enabled" is not
		 * explicitly set.
		 */
		attr = "true";

	attr = attr ? (!strcmp(attr, "true") ? " up" : " down") : "";
	fprintf(ip, "%s", attr);

	/* Type specific attributes */

	fputc('\n', ip);

	/* IP Addresses */

	return SR_ERR_OK;
}

static int ifchange(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *diff, *cifs, *difs, *iface;
	struct confd *confd = _confd;
	sr_data_t *cfg;
	sr_error_t err;

	switch (event) {
	case SR_EV_CHANGE:
		break;
	case SR_EV_ABORT:
		return netdo_abort(&confd->netdo);
	case SR_EV_DONE:
		err = netdo_done(&confd->netdo);
		if (err)
			return err;

		return systemf("%s", "net do");
	default:
		return SR_ERR_OK;
	}

	err = sr_get_data(session, "/interfaces/interface//.", 0, 0, 0, &cfg);
	if (err)
		return err;

	err = srx_get_diff(session, (struct lyd_node **)&diff);
	if (err)
		goto err_release_data;

	cifs = lydx_get_descendant(cfg->tree, "interfaces", "interface", NULL);
	difs = lydx_get_descendant(diff, "interfaces", "interface", NULL);

	err = netdo_change(&confd->netdo, cifs, difs);
	if (err)
		goto err_free_diff;

	LY_LIST_FOR(difs, iface) {
		err = ifchange_one(confd, iface);
		if (err) {
			netdo_abort(&confd->netdo);
			break;
		}
	}

err_free_diff:
	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
	return err;
}

int ietf_interfaces_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, ietf_if_reqs);
	if (rc)
		goto fail;

	rc = netdo_boot();
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, "ietf-interfaces", "/ietf-interfaces:interfaces", 0, ifchange, confd, &confd->sub);

	sr_session_switch_ds(confd->session, SR_DS_CANDIDATE);
	REGISTER_CHANGE(confd->session, "ietf-interfaces", "/ietf-interfaces:interfaces",
			SR_SUBSCR_UPDATE, ifchange_cand, confd, &confd->sub);

	sr_session_switch_ds(confd->session, SR_DS_RUNNING);
	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
