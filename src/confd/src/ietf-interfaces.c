/* SPDX-License-Identifier: BSD-3-Clause */

#include <fnmatch.h>
#include <stdbool.h>

#include <jansson.h>

#include <net/if.h>

#include "core.h"
#include "srx_module.h"
#include "srx_val.h"

static const char *iffeat[] = {
	"if-mib",
	NULL
};

static const char *ipfeat[] = {
	"ipv4-non-contiguous-netmasks",
	NULL
};

static const struct srx_module_requirement ietf_if_reqs[] = {
	{ .dir = YANG_PATH_, .name = "ietf-interfaces", .rev = "2018-02-20", .features = iffeat },
	{ .dir = YANG_PATH_, .name = "iana-if-type", .rev = "2023-01-26" },
	{ .dir = YANG_PATH_, .name = "ietf-ip", .rev = "2018-02-22", .features = ipfeat },
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

	proc = popenf("re", "ip -d -j link show dev %s", ifname);
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

static int ifchange(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	return SR_ERR_OK;
}

int ietf_interfaces_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, ietf_if_reqs);
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
