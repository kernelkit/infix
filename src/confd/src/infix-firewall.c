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

#define MODULE                 "infix-firewall"
#define CFG_XPATH              "/infix-firewall:firewall"

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

static int generate_zone(const char *name, struct lyd_node *cfg)
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
	fprintf(fp, "  <short>%s</short>", name);
	if (desc)
		fprintf(fp, "  <description>%s</description>\n", desc);
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "interfaces")
		fprintf(fp, "  <interface name=\"%s\"/>\n", lyd_get_value(node));
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "sources")
		fprintf(fp, "  <source address=\"%s\"/>\n", lyd_get_value(node));
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "services")
		fprintf(fp, "  <service name=\"%s\"/>\n", lyd_get_value(node));
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "forward") {
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
	
	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "icmp-blocks") {
		const char *icmp_type = lydx_get_cattr(node, "icmp-type");
		fprintf(fp, "  <icmp-block name=\"%s\"/>\n", icmp_type);
	}
	
	if (lydx_is_enabled(cfg, "forwarding"))
		fprintf(fp, "  <forward/>\n");
	if (lydx_is_enabled(cfg, "masquerade"))
		fprintf(fp, "  <masquerade/>\n");
	
	fprintf(fp, "</zone>\n");
	
	return close_file(fp);
}

static int generate_service(const char *name, struct lyd_node *service_cfg)
{
	const char *desc, *destination;
	struct lyd_node *node;
	FILE *fp;
	
	fp = open_file(FIREWALLD_SERVICES_DIR, name);
	if (!fp)
		return SR_ERR_SYS;
	
	desc = lydx_get_cattr(service_cfg, "description");
	destination = lydx_get_cattr(service_cfg, "destination");
	
	fprintf(fp, "<service>\n");
	
	if (desc)
		fprintf(fp, "  <short>%s</short>\n", desc);
	
	if (destination)
		fprintf(fp, "  <destination ipv4=\"%s\"/>\n", destination);
	
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
	const char *default_zone, *logging;
	FILE *fp;

	fp = fopen(FIREWALLD_CONF, "w");
	if (!fp) {
		ERRNO("Failed creating %s", FIREWALLD_CONF);
		return SR_ERR_SYS;
	}

	default_zone = lydx_get_cattr(tree, "default");
	logging = lydx_get_cattr(tree, "logging");

	fprintf(fp, "DefaultZone=%s\n", default_zone ? default_zone : "public");
	fprintf(fp, "MinimalMark=100\n");
	fprintf(fp, "CleanupOnExit=yes\n");
	fprintf(fp, "Lockdown=no\n");
	fprintf(fp, "IPv6_rpfilter=yes\n");
	fprintf(fp, "IndividualCalls=no\n");
	fprintf(fp, "LogDenied=%s\n", logging ? logging : "off");
	fprintf(fp, "AutomaticHelpers=system\n");
	fprintf(fp, "FirewallBackend=nftables\n");
	fprintf(fp, "FlushAllOnReload=yes\n");
	fprintf(fp, "RFC3964_IPv4=yes\n");
	fclose(fp);

	return SR_ERR_OK;
}

static int create_default_zone(sr_session_ctx_t *session, const char *name,
			      const char *policy, const char *desc,
			      const char *services[])
{
	char xpath[256];
	int rc;

	snprintf(xpath, sizeof(xpath), CFG_XPATH "/zones/zone[name='%s']/name", name);
	rc = sr_set_item_str(session, xpath, name, NULL, 0);
	if (rc) return rc;

	snprintf(xpath, sizeof(xpath), CFG_XPATH "/zones/zone[name='%s']/description", name);
	rc = sr_set_item_str(session, xpath, desc, NULL, 0);
	if (rc) return rc;

	snprintf(xpath, sizeof(xpath), CFG_XPATH "/zones/zone[name='%s']/policy", name);
	rc = sr_set_item_str(session, xpath, policy, NULL, 0);
	if (rc) return rc;

	for (int i = 0; services && services[i]; i++) {
		snprintf(xpath, sizeof(xpath), CFG_XPATH "/zones/zone[name='%s']/services[.='%s']", name, services[i]);
		rc = sr_set_item_str(session, xpath, services[i], NULL, 0);
		if (rc) return rc;
	}

	return SR_ERR_OK;
}

static int infer_default_zones(sr_session_ctx_t *session)
{
	int rc;

	const char *internal_services[] = {"ssh", "dns", "http", "https", NULL};
	rc = create_default_zone(session, "internal", "accept", "Internal trusted network", internal_services);
	if (rc) return rc;

	rc = create_default_zone(session, "external", "drop", "External untrusted network", NULL);
	if (rc) return rc;

	const char *dmz_services[] = {"http", "https", NULL};
	rc = create_default_zone(session, "dmz", "reject", "Demilitarized zone", dmz_services);
	if (rc) return rc;

	const char *public_services[] = {"ssh", NULL};
	rc = create_default_zone(session, "public", "reject", "Public access zone", public_services);
	if (rc) return rc;

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

	err = sr_get_data(session, CFG_XPATH "//.", 0, 0, 0, &cfg);
	if (err || !cfg)
		return SR_ERR_INTERNAL;
	
	tree = cfg->tree;
	global = lydx_get_descendant(tree, "firewall", NULL);
	
	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;
	
	if (!diff)
		goto err_release_data;

	if (lydx_is_enabled(global, "enabled")) {
		sr_data_t *zones_data;

		system("initctl -nbq enable firewalld");

		if (!sr_get_data(session, CFG_XPATH "/zones", 0, 0, 0, &zones_data) &&
		    (!zones_data || !zones_data->tree)) {
			infer_default_zones(session);
			sr_apply_changes(session, 0);
		}
		if (zones_data)
			sr_release_data(zones_data);
	} else {
		system("initctl -nbq disable firewalld");
		goto done;
	}

	if (lydx_get_descendant(diff, "firewall", "default", NULL) ||
	    lydx_get_descendant(diff, "firewall", "logging", NULL)) {
		generate_firewalld_conf(tree);
		reload_needed = true;
	}

	list = lydx_get_descendant(diff, "firewall", "zones", "zone", NULL);
	LYX_LIST_FOR_EACH(list, node, "zone") {
		const char *name = lydx_get_cattr(node, "name");
			
		if (lydx_get_op(node) == LYDX_OP_DELETE) {
			delete_file(FIREWALLD_ZONES_DIR, name);
			reload_needed = true;
			continue;
		}

		clist = lydx_get_descendant(tree, "firewall", "zones", "zone", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "zone") {
			if (strcmp(name, lydx_get_cattr(cnode, "name")))
				continue;

			generate_zone(name, cnode);
			reload_needed = true;
			break;
		}
	}
	
	list = lydx_get_descendant(diff, "firewall", "services", "service", NULL);
	LYX_LIST_FOR_EACH(list, node, "service") {
		const char *name = lydx_get_cattr(node, "name");
			
		if (lydx_get_op(node) == LYDX_OP_DELETE) {
			delete_file(FIREWALLD_SERVICES_DIR, name);
			reload_needed = true;
			continue;
		}

		clist = lydx_get_descendant(tree, "firewall", "services", "service", NULL);
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
	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
	
	return err;
}

int infix_firewall_init(struct confd *confd)
{
	int rc;
	
	mkdir(FIREWALLD_DIR, 0755);
	mkdir(FIREWALLD_ZONES_DIR, 0755);
	mkdir(FIREWALLD_SERVICES_DIR, 0755);
	mkdir(FIREWALLD_POLICIES_DIR, 0755);
	
	sr_apply_changes(confd->session, 0);

	REGISTER_CHANGE(confd->session, MODULE, CFG_XPATH, 0, change, confd, &confd->sub);
	
	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
