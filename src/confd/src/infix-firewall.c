/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/stat.h>
#include <errno.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>
#include <libyang/libyang.h>

#include "core.h"
#include "ietf-interfaces.h"

#define MODULE                 "infix-firewall"
#define XPATH                  "/infix-firewall:firewall"

#define FIREWALLD_DIR          "/etc/firewalld"
#define FIREWALLD_CONF         FIREWALLD_DIR "/firewalld.conf"
#define FIREWALLD_ZONES_DIR    FIREWALLD_DIR "/zones"
#define FIREWALLD_SERVICES_DIR FIREWALLD_DIR "/services"
#define FIREWALLD_POLICIES_DIR FIREWALLD_DIR "/policies"

static struct {
	const char *yang;
	const char *target;
} zone_policy_map[] = {
	{ "reject", "%%REJECT%%" },
	{ "accept", "ACCEPT" },
	{ "drop",   "DROP" },
};

static struct {
	const char *yang;
	const char *target;
} policy_action_map[] = {
	{ "continue", "CONTINUE" },
	{ "accept",   "ACCEPT" },
	{ "reject",   "REJECT" },
	{ "drop",     "DROP" },
};

static const char *zone_policy_to_target(const char *policy)
{
	for (size_t i = 0; policy && i < NELEMS(zone_policy_map); i++) {
		if (!strcmp(policy, zone_policy_map[i].yang))
			return zone_policy_map[i].target;
	}
	
	return zone_policy_map[0].yang;
}

static const char *policy_action_to_target(const char *action)
{
	for (size_t i = 0; action && i < NELEMS(policy_action_map); i++) {
		if (!strcmp(action, policy_action_map[i].yang))
			return policy_action_map[i].target;
	}
	
	return policy_action_map[0].yang;
}

static void mark_interfaces_used(struct lyd_node *cfg, char **l3_ifaces)
{
	struct lyd_node *node;

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "interfaces") {
		const char *ifname = lyd_get_value(node);

		for (int i = 0; l3_ifaces[i]; i++) {
			if (!strcmp(l3_ifaces[i], ifname)) {
				l3_ifaces[i][0] = '\0';
				break;
			}
		}
	}
}


static FILE *open_file(const char *dir, const char *name)
{
	FILE *fp;
	
	fp = fopenf("w", "%s/%s.xml", dir, name);
	if (!fp) {
		ERRNO("Failed creating %s/%s.xml: %s", dir, name, strerror(errno));
		return NULL;
	}
	
	fprintf(fp, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n");
	return fp;
}

static int close_file(FILE *fp)
{
	fclose(fp);
	return SR_ERR_OK;
}

static int delete_file(const char *dir, const char *name)
{
	if (erasef("%s/%s.xml", dir, name) && errno != ENOENT) {
		ERRNO("Failed deleting %s/%s.xml: %s", dir, name, strerror(errno));
		return SR_ERR_SYS;
	}
	
	return SR_ERR_OK;
}

static int generate_zone_with_extra_interfaces(const char *name, struct lyd_node *cfg, char **extra_ifaces)
{
	const char *policy, *desc;
	struct lyd_node *node;
	FILE *fp;
	
	fp = open_file(FIREWALLD_ZONES_DIR, name);
	if (!fp)
		return SR_ERR_SYS;
	
	policy = lydx_get_cattr(cfg, "policy");
	desc = lydx_get_cattr(cfg, "description");
	
	fprintf(fp, "<zone target=\"%s\">\n", zone_policy_to_target(policy));
	fprintf(fp, "  <short>%s</short>\n", name);
	if (desc)
		fprintf(fp, "  <description>%s</description>\n", desc);
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "interfaces")
		fprintf(fp, "  <interface name=\"%s\"/>\n", lyd_get_value(node));

	if (extra_ifaces) {
		for (int i = 0; extra_ifaces[i]; i++) {
			if (extra_ifaces[i][0] != '\0') {
				fprintf(fp, "  <interface name=\"%s\"/>\n", extra_ifaces[i]);
			}
		}
	}
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "sources")
		fprintf(fp, "  <source address=\"%s\"/>\n", lyd_get_value(node));
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "services")
		fprintf(fp, "  <service name=\"%s\"/>\n", lyd_get_value(node));
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "port-forward") {
		const char *port = lydx_get_cattr(node, "port");
		const char *proto = lydx_get_cattr(node, "proto");
		struct lyd_node *to = lydx_get_child(node, "to");
		
		if (to) {
			const char *to_addr = lydx_get_cattr(to, "addr");
			const char *to_port = lydx_get_cattr(to, "port");
			
			fprintf(fp, "  <forward-port port=\"%s\" protocol=\"%s\"", port, proto);
			
			if (to_addr)
				fprintf(fp, " to-addr=\"%s\"", to_addr);
			if (to_port)
				fprintf(fp, " to-port=\"%s\"", to_port);
			
			fprintf(fp, "/>\n");
		}
	}
	
#if 0 /* ADVANCED FEATURE: icmp-blocks removed from YANG model */
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "icmp-blocks") {
		const char *icmp_type = lydx_get_cattr(node, "icmp-type");
		fprintf(fp, "  <icmp-block name=\"%s\"/>\n", icmp_type);
	}
#endif
	
	if (lydx_is_enabled(cfg, "forwarding"))
		fprintf(fp, "  <forward/>\n");
#if 0 /* REMOVED: Zone-level masquerade - handled by policy rules instead */
	if (lydx_is_enabled(cfg, "masquerade"))
		fprintf(fp, "  <masquerade/>\n");
#endif
	
	fprintf(fp, "</zone>\n");
	
	return close_file(fp);
}

static int generate_zone(const char *name, struct lyd_node *cfg)
{
	return generate_zone_with_extra_interfaces(name, cfg, NULL);
}

static int generate_default_zone_with_remaining_interfaces(struct lyd_node *tree, char **l3_ifaces)
{
	const char *default_zone = lydx_get_cattr(lydx_get_descendant(tree, "firewall", NULL), "default");
	struct lyd_node *zones, *zone_cfg = NULL;
	int unassigned_count = 0;
	
	for (int i = 0; l3_ifaces[i]; i++) {
		if (l3_ifaces[i][0] != '\0') {
			unassigned_count++;
		}
	}
	
	if (unassigned_count == 0)
		return 0;
	
	zones = lydx_get_descendant(tree, "firewall", "zone", NULL);
	LYX_LIST_FOR_EACH(zones, zone_cfg, "zone") {
		const char *name = lydx_get_cattr(zone_cfg, "name");
		if (!strcmp(name, default_zone))
			break;
	}
	
	ERROR("Adding %d unassigned interfaces to default zone '%s':", unassigned_count, default_zone);
	for (int i = 0; l3_ifaces[i]; i++) {
		if (l3_ifaces[i][0] != '\0') {
			ERROR("  - %s", l3_ifaces[i]);
		}
	}
	
	return generate_zone_with_extra_interfaces(default_zone, zone_cfg, l3_ifaces);
}

static int generate_service(const char *name, struct lyd_node *service_cfg)
{
	const char *desc;
#if 0 /* ADVANCED FEATURE: destination variable for service destinations */
	const char *destination;
#endif
	struct lyd_node *node;
	FILE *fp;
	
	fp = open_file(FIREWALLD_SERVICES_DIR, name);
	if (!fp)
		return SR_ERR_SYS;
	
	desc = lydx_get_cattr(service_cfg, "description");
#if 0 /* ADVANCED FEATURE: service destinations removed from YANG model */
	destination = lydx_get_cattr(service_cfg, "destination");
#endif
	
	fprintf(fp, "<service>\n");
	
	if (desc)
		fprintf(fp, "  <short>%s</short>\n", desc);
	
#if 0 /* ADVANCED FEATURE: service destinations removed from YANG model */
	if (destination)
		fprintf(fp, "  <destination ipv4=\"%s\"/>\n", destination);
#endif
	
	LYX_LIST_FOR_EACH(lyd_child(service_cfg), node, "port") {
		const char *lower = lydx_get_cattr(node, "lower");
		const char *upper = lydx_get_cattr(node, "upper");
		const char *proto = lydx_get_cattr(node, "proto");
		
		if (upper && strcmp(lower, upper))
			fprintf(fp, "  <port port=\"%s-%s\" protocol=\"%s\"/>\n", lower, upper, proto);
		else
			fprintf(fp, "  <port port=\"%s\" protocol=\"%s\"/>\n", lower, proto);
	}
	
	fprintf(fp, "</service>\n");
	
	return close_file(fp);
}

static int generate_policy(const char *name, struct lyd_node *policy_cfg)
{
	const char *desc, *policy;
	struct lyd_node *node;
	bool masquerade;
	FILE *fp;
	
	fp = open_file(FIREWALLD_POLICIES_DIR, name);
	if (!fp)
		return SR_ERR_SYS;
	
	desc = lydx_get_cattr(policy_cfg, "description");
	policy = lydx_get_cattr(policy_cfg, "policy");
	masquerade = lydx_is_enabled(policy_cfg, "masquerade");
	
	fprintf(fp, "<policy target=\"%s\">\n", policy_action_to_target(policy));
	
	if (desc)
		fprintf(fp, "  <description>%s</description>\n", desc);
	
	LYX_LIST_FOR_EACH(lyd_child(policy_cfg), node, "ingress")
		fprintf(fp, "  <ingress-zone name=\"%s\"/>\n", lyd_get_value(node));
	
	LYX_LIST_FOR_EACH(lyd_child(policy_cfg), node, "egress")
		fprintf(fp, "  <egress-zone name=\"%s\"/>\n", lyd_get_value(node));
	
	LYX_LIST_FOR_EACH(lyd_child(policy_cfg), node, "service")
		fprintf(fp, "  <service name=\"%s\"/>\n", lyd_get_value(node));
	
	if (masquerade)
		fprintf(fp, "  <masquerade/>\n");
	
	fprintf(fp, "</policy>\n");
	
	return close_file(fp);
}

static int generate_firewalld_conf(struct lyd_node *tree)
{
	FILE *fp;

	fp = fopen(FIREWALLD_CONF, "w");
	if (!fp) {
		ERRNO("Failed creating %s", FIREWALLD_CONF);
		return SR_ERR_SYS;
	}

	fprintf(fp, "DefaultZone=%s\n", lydx_get_cattr(tree, "default"));
	fprintf(fp, "MinimalMark=100\n");
	fprintf(fp, "CleanupOnExit=yes\n");
	fprintf(fp, "Lockdown=no\n");
	fprintf(fp, "IPv6_rpfilter=yes\n");
	fprintf(fp, "IndividualCalls=no\n");
	fprintf(fp, "LogDenied=%s\n", lydx_get_cattr(tree, "logging") ?: "off");
	fprintf(fp, "AutomaticHelpers=system\n");
	fprintf(fp, "FirewallBackend=nftables\n");
	fprintf(fp, "FlushAllOnReload=yes\n");
	fprintf(fp, "RFC3964_IPv4=yes\n");
	fclose(fp);

	return SR_ERR_OK;
}

static int infer_zone(sr_session_ctx_t *session, const char *name, const char *desc,
		      const char *policy, bool forwarding, const char *services[])
{
	int rc;

	ERROR("Inferring zone %s (%s), policy %s forwarding %d", name, desc, policy, forwarding);

	rc = srx_set_str(session, name, 0, XPATH "/zone[name='%s']/name", name);
	if (rc)
		return rc;

	rc = srx_set_str(session, desc, 0, XPATH "/zone[name='%s']/description", name);
	if (rc)
		return rc;

	rc = srx_set_str(session, policy, 0, XPATH "/zone[name='%s']/policy", name);
	if (rc)
		return rc;

	rc = srx_set_bool(session, forwarding, 0, XPATH "/zone[name='%s']/forwarding", name);
	if (rc)
		return rc;

	for (int i = 0; services && services[i]; i++) {
		rc = srx_set_str(session, services[i], 0, XPATH "/zone[name='%s']/services[.='%s']", name, services[i]);
		if (rc)
			return rc;
	}

	return SR_ERR_OK;
}

static int change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		  const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *diff, *tree, *list, *node, *global;
	struct lyd_node *clist, *cnode;
	bool reload_needed = false;
	sr_error_t err = SR_ERR_OK;
	sr_data_t *cfg;

	switch (event) {
	case SR_EV_CHANGE:
		/* Validation phase - just return OK for now */
		return SR_ERR_OK;

	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;

	case SR_EV_DONE:
		break;
	}

	err = sr_get_data(session, "//.", 0, 0, 0, &cfg);
	if (err || !cfg)
		return SR_ERR_INTERNAL;

	tree = cfg->tree;
	global = lydx_get_descendant(tree, "firewall", NULL);

	/* Get L3 interfaces for default zone assignment */
	char **l3_ifaces = NULL;
	if (ietf_interfaces_get_all_l3(tree, &l3_ifaces) != 0) {
		ERROR("Failed to get L3 interfaces");
		l3_ifaces = NULL;
	}
	
	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;

	if (!diff)
		goto err_release_data;

	if (!global)
		goto done;

	if (lydx_get_descendant(diff, "firewall", "default", NULL) ||
	    lydx_get_descendant(diff, "firewall", "logging", NULL)) {
		generate_firewalld_conf(global);
		reload_needed = true;
	}

	/* Stage 1: Generate explicit zones (skip default zone) */
	const char *default_zone = lydx_get_cattr(global, "default");
	list = lydx_get_descendant(diff, "firewall", "zone", NULL);
	LYX_LIST_FOR_EACH(list, node, "zone") {
		const char *name = lydx_get_cattr(node, "name");
			
		if (lydx_get_op(node) == LYDX_OP_DELETE) {
			delete_file(FIREWALLD_ZONES_DIR, name);
			reload_needed = true;
			continue;
		}

		clist = lydx_get_descendant(tree, "firewall", "zone", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "zone") {
			if (strcmp(name, lydx_get_cattr(cnode, "name")))
				continue;

			/* Skip default zone in stage 1 */
			if (!strcmp(name, default_zone)) {
				if (l3_ifaces)
					mark_interfaces_used(cnode, l3_ifaces);
				break;
			}

			generate_zone(name, cnode);
			if (l3_ifaces)
				mark_interfaces_used(cnode, l3_ifaces);
			reload_needed = true;
			break;
		}
	}
	
	/* Stage 2: Generate default zone with remaining interfaces */
	if (l3_ifaces && generate_default_zone_with_remaining_interfaces(tree, l3_ifaces) == 0)
		reload_needed = true;
	
	list = lydx_get_descendant(diff, "firewall", "service", NULL);
	LYX_LIST_FOR_EACH(list, node, "service") {
		const char *name = lydx_get_cattr(node, "name");
			
		if (lydx_get_op(node) == LYDX_OP_DELETE) {
			delete_file(FIREWALLD_SERVICES_DIR, name);
			reload_needed = true;
			continue;
		}

		clist = lydx_get_descendant(tree, "firewall", "service", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "service") {
			if (strcmp(name, lydx_get_cattr(cnode, "name")))
				continue;

			generate_service(name, cnode);
			reload_needed = true;
			break;
		}
	}
	
	list = lydx_get_descendant(diff, "firewall", "policy", NULL);
	LYX_LIST_FOR_EACH(list, node, "policy") {
		const char *name = lydx_get_cattr(node, "name");
			
		if (lydx_get_op(node) == LYDX_OP_DELETE) {
			delete_file(FIREWALLD_POLICIES_DIR, name);
			reload_needed = true;
			continue;
		}

		clist = lydx_get_descendant(tree, "firewall", "policy", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "policy") {
			if (strcmp(name, lydx_get_cattr(cnode, "name")))
				continue;

			generate_policy(name, cnode);
			reload_needed = true;
			break;
		}
	}

	if (reload_needed)
		system("initctl -nbq touch firewalld");
	
done:
	systemf("initctl -nbq %s firewalld", global ? "enable" : "disable");

	if (l3_ifaces) {
		for (int i = 0; l3_ifaces[i]; i++)
			free(l3_ifaces[i]);
		free(l3_ifaces);
	}

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
	
	return err;
}

/*
 * Set up default zones with sane defaults:
 *
 *  internal: This is the default zone, which trusts all ingressing traffic.
 *            It is used for internal trusted networks and allows forwarding
 *            between all interfaces and networks in the zone.
 *
 *  external: Untrusted zone, for WAN interfaces, only ssh and dhcpv6 client
 *            traffic is allowed to ingress.
 *
 */
static int cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		const char *path, sr_event_t event, unsigned request_id, void *priv)
{
	const char *ext_svc[] = {"ssh", "dhcpv6-client", NULL};
	const char *int_svc[] = { NULL};
	size_t cnt = 0;
	int rc;

	if (event != SR_EV_UPDATE && event != SR_EV_CHANGE)
		return 0;

	/* If unset, this is the first time we're called */
	if (srx_get_str(session, XPATH "/default"))
		return 0;

	rc = srx_nitems(session, &cnt, XPATH "/zones");
	if (rc || cnt) {
		WARN("firewall has zones defined %zu, but no default zone for new interfaces! (rc %d)",
		     cnt, rc);
		return 0;
	}

	rc = infer_zone(session, "external", "External untrusted network, only SSH and DHCPv6 client.",
			"drop", true, ext_svc);
	if (rc)
		return rc;

	rc = infer_zone(session, "internal", "Internal trusted network, forwarding between networks.",
			"accept", true, int_svc);
	if (rc)
		return rc;

	/* Set up default zone for new networks */
	rc = srx_set_str(session, "internal", 0, XPATH "/default");
	if (rc)
		return rc;

	return SR_ERR_OK;
}

int infix_firewall_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->session, MODULE, XPATH, 0, change, confd, &confd->sub);
	REGISTER_CHANGE(confd->cand, MODULE, XPATH "//.", SR_SUBSCR_UPDATE, cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
