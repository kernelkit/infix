/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <dirent.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>
#include <libyang/libyang.h>

#include "core.h"
#include "interfaces.h"

#define MODULE                 "infix-firewall"
#define XPATH                  "/infix-firewall:firewall"
#define INFER_POLICY           0
#define FIREWALLD_DIR          "/etc/firewalld"
#define FIREWALLD_DIR_NEXT     "/etc/firewalld+"
#define FIREWALLD_CONF         FIREWALLD_DIR_NEXT "/firewalld.conf"
#define FIREWALLD_ZONES_DIR    FIREWALLD_DIR_NEXT "/zones"
#define FIREWALLD_SERVICES_DIR FIREWALLD_DIR_NEXT "/services"
#define FIREWALLD_POLICIES_DIR FIREWALLD_DIR_NEXT "/policies"

static struct {
	const char *yang;
	const char *target;
} zone_action_map[] = {
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

static const char *zone_action_to_target(const char *action)
{
	for (size_t i = 0; action && i < NELEMS(zone_action_map); i++) {
		if (!strcmp(action, zone_action_map[i].yang))
			return zone_action_map[i].target;
	}

	return zone_action_map[0].yang;
}

static const char *policy_action_to_target(const char *action)
{
	for (size_t i = 0; action && i < NELEMS(policy_action_map); i++) {
		if (!strcmp(action, policy_action_map[i].yang))
			return policy_action_map[i].target;
	}

	return policy_action_map[0].yang;
}

static void mark_interfaces_used(struct lyd_node *cfg, char **ifaces)
{
	struct lyd_node *node;

	if (!ifaces)
		return;

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "interface") {
		const char *ifname = lyd_get_value(node);

		for (int i = 0; ifaces[i]; i++) {
			if (!strcmp(ifaces[i], ifname)) {
				ifaces[i][0] = '\0';
				break;
			}
		}
	}
}

static void log_unzoned(const char *name, char **ifaces)
{
	size_t num = 0;

	for (int i = 0; ifaces && ifaces[i]; i++) {
		if (ifaces[i][0] != '\0')
			num++;
	}

	if (num > 0) {
		size_t sz = num * 16 + 2 * num + 1;
		char buf[sz];
		int hit = 0;

		memset(buf, 0, sz);
		for (int i = 0; ifaces[i]; i++) {
			if (ifaces[i][0] == '\0')
				continue;
			if (hit)
				strlcat(buf, ", ", sz);
			strlcat(buf, ifaces[i], sz);
			hit++;
		}

		WARN("Adding %zu unassigned interfaces to default zone '%s': %s",
		     num, name, buf);
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

static int generate_zone(struct lyd_node *cfg, const char *name, char **ifaces)
{
	const char *action, *desc;
	struct lyd_node *node;
	FILE *fp;

	fp = open_file(FIREWALLD_ZONES_DIR, name);
	if (!fp)
		return SR_ERR_SYS;

	action = lydx_get_cattr(cfg, "action");
	desc = lydx_get_cattr(cfg, "description");

	fprintf(fp, "<zone target=\"%s\">\n", zone_action_to_target(action));
	fprintf(fp, "  <short>%s</short>\n", name);

	if (desc)
		fprintf(fp, "  <description>%s</description>\n", desc);

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "interface")
		fprintf(fp, "  <interface name=\"%s\"/>\n", lyd_get_value(node));

	if (ifaces) {
		for (int i = 0; ifaces[i]; i++) {
			if (ifaces[i][0] != '\0') {
				fprintf(fp, "  <interface name=\"%s\"/>\n", ifaces[i]);
			}
		}

		log_unzoned(name, ifaces);
	}

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "network")
		fprintf(fp, "  <source address=\"%s\"/>\n", lyd_get_value(node));

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "service")
		fprintf(fp, "  <service name=\"%s\"/>\n", lyd_get_value(node));

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "port-forward") {
		const char *lower = lydx_get_cattr(node, "lower");
		const char *upper = lydx_get_cattr(node, "upper");
		const char *proto = lydx_get_cattr(node, "proto");
		struct lyd_node *to = lydx_get_child(node, "to");

		if (to) {
			const char *to_addr = lydx_get_cattr(to, "addr");
			const char *to_port = lydx_get_cattr(to, "port");

			if (upper) {
				/* Port range */
				fprintf(fp, "  <forward-port port=\"%s-%s\" protocol=\"%s\"", lower, upper, proto);

				if (to_addr)
					fprintf(fp, " to-addr=\"%s\"", to_addr);
				if (to_port)
					fprintf(fp, " to-port=\"%s\"", to_port);

				fprintf(fp, "/>\n");
			} else {
				/* Single port */
				fprintf(fp, "  <forward-port port=\"%s\" protocol=\"%s\"", lower, proto);

				if (to_addr)
					fprintf(fp, " to-addr=\"%s\"", to_addr);
				if (to_port)
					fprintf(fp, " to-port=\"%s\"", to_port);

				fprintf(fp, "/>\n");
			}
		}
	}

	fprintf(fp, "</zone>\n");

	return close_file(fp);
}

static int generate_service(struct lyd_node *cfg, const char *name)
{
	const char *desc;
	const char *dest;
	struct lyd_node *node;
	FILE *fp;

	fp = open_file(FIREWALLD_SERVICES_DIR, name);
	if (!fp)
		return SR_ERR_SYS;

	desc = lydx_get_cattr(cfg, "description");
	dest = lydx_get_cattr(cfg, "destination");

	fprintf(fp, "<service>\n");

	if (desc)
		fprintf(fp, "  <short>%s</short>\n", desc);

	if (dest)
		fprintf(fp, "  <destination ipv%s=\"%s\"/>\n", strchr(dest, ':') ? "6" : "4", dest);

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "port") {
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

static int generate_policy(struct lyd_node *cfg, const char *name, int *priority)
{
	const char *desc, *action;
	struct lyd_node *node;
	bool masquerade;
	FILE *fp;

	if (*priority > 0) {
		ERROR("Too many policies/filters - exceeded int16 range");
		return SR_ERR_SYS;
	}

	fp = open_file(FIREWALLD_POLICIES_DIR, name);
	if (!fp)
		return SR_ERR_SYS;

	desc = lydx_get_cattr(cfg, "description");
	action = lydx_get_cattr(cfg, "action");
	masquerade = lydx_is_enabled(cfg, "masquerade");

	fprintf(fp, "<policy target=\"%s\" priority=\"%d\">\n",
		policy_action_to_target(action), (*priority)++);

	if (desc)
		fprintf(fp, "  <description>%s</description>\n", desc);

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "ingress")
		fprintf(fp, "  <ingress-zone name=\"%s\"/>\n", lyd_get_value(node));

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "egress")
		fprintf(fp, "  <egress-zone name=\"%s\"/>\n", lyd_get_value(node));

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "service")
		fprintf(fp, "  <service name=\"%s\"/>\n", lyd_get_value(node));

	/* Handle custom filters */
	node = lydx_get_descendant(cfg, "policy", "custom", NULL);
	if (node) {
		struct lyd_node *filter;

		LYX_LIST_FOR_EACH(lyd_child(node), filter, "filter") {
			const char *family = lydx_get_cattr(filter, "family");
			struct lyd_node *icmp;

			if (*priority > 0) {
				ERROR("Too many policies/filters - exceeded int16 range");
				close_file(fp);
				delete_file(FIREWALLD_POLICIES_DIR, name);
				return SR_ERR_SYS;
			}

			if (strcmp(family, "both"))
				fprintf(fp, "    <rule family=\"%s\" priority=\"%d\">\n",
					family, (*priority)++);
			else
				fprintf(fp, "    <rule priority=\"%d\">\n", (*priority)++);

			action = lydx_get_cattr(filter, "action");
			icmp = lydx_get_descendant(filter, "filter", "icmp", NULL);
			if (icmp) {
				const char *type = lydx_get_cattr(icmp, "type");

				if (strcmp(action, "reject") == 0) {
					fprintf(fp, "     <icmp-block name=\"%s\"/>\n", type);
				} else {
					fprintf(fp, "     <icmp-type name=\"%s\"/>\n", type);
					fprintf(fp, "     <%s/>\n", action);
				}
			}

			fprintf(fp, "   </rule>\n");
		}
	}

	if (masquerade)
		fprintf(fp, "  <masquerade/>\n");

	fprintf(fp, "</policy>\n");

	return close_file(fp);
}

static int generate_firewalld_conf(struct lyd_node *cfg)
{
	FILE *fp;

	fp = fopen(FIREWALLD_CONF, "w");
	if (!fp) {
		ERRNO("Failed creating %s", FIREWALLD_CONF);
		return SR_ERR_SYS;
	}

	fprintf(fp, "DefaultZone=%s\n", lydx_get_cattr(cfg, "default"));
	fprintf(fp, "LogDenied=%s\n", lydx_get_cattr(cfg, "logging") ?: "off");

	fprintf(fp, "FirewallBackend=nftables\n");
	fprintf(fp, "IndividualCalls=no\n");

	/*
	 * Set nftables rule set to be owned exclusively by firewalld.
	 * This prevents other entities from mistakenly (or maliciously)
	 * modifying firewalld's rule set -- e.g., 'nft flush ruleset'
	 * will not affect the firewalld rules.
	 */
	fprintf(fp, "NftablesTableOwner=yes\n");

	/* TODO: add config option to enable nftables flowtable (fastpath) */
	fprintf(fp, "NftablesFlowtable=off\n");

	/* TODO: Add config option to enable this useful debug option. */
	fprintf(fp, "NftablesCounters=no\n");

	/* Drop all traffic, except established connections, while rules are updated */
	fprintf(fp, "ReloadPolicy=INPUT:DROP,FORWARD:DROP,OUTPUT:DROP\n");
	fprintf(fp, "FlushAllOnReload=yes\n");

	/* Seamless integration with podman -- published ports are opened. */
	fprintf(fp, "StrictForwardPorts=no\n");

	/* Performs reverse path filtering (RPF) on IPv6 packets as per RFC 3704 */
	fprintf(fp, "IPv6_rpfilter=loose-forward\n");

	/*
	 * Filter IPv6 traffic with 6to4 destination addresses that correspond
	 * to IPv4 addresses that should not be routed over the public internet.
	 */
	fprintf(fp, "RFC3964_IPv4=yes\n");

	/* Remove all firewall rules on exit */
	fprintf(fp, "CleanupOnExit=yes\n");
	fclose(fp);

	return SR_ERR_OK;
}

static int infer_zone(sr_session_ctx_t *session, const char *name, const char *desc,
		      const char *action, const char *services[])
{
	int rc;

	DEBUG("Inferring zone %s (%s), action %s", name, desc, action);

	rc = srx_set_str(session, desc, 0, XPATH "/zone[name='%s']/description", name);
	if (rc)
		return rc;

	rc = srx_set_str(session, action, 0, XPATH "/zone[name='%s']/action", name);
	if (rc)
		return rc;

	for (int i = 0; services && services[i]; i++) {
		rc = srx_set_str(session, services[i], 0, XPATH "/zone[name='%s']/service[.='%s']",
				 name, services[i]);
		if (rc)
			return rc;
	}

	return SR_ERR_OK;
}

#if INFER_POLICY
static int infer_policy(sr_session_ctx_t *session, const char *name, const char *desc,
			const char *action, const char *ingress[], const char *egress[],
			const char *icmp_types[][4])
{
	int rc;

	DEBUG("Inferring policy %s (%s), action %s", name, desc, action);

	rc = srx_set_str(session, desc, 0, XPATH "/policy[name='%s']/description", name);
	if (rc)
		return rc;

	rc = srx_set_str(session, action, 0, XPATH "/policy[name='%s']/action", name);
	if (rc)
		return rc;

	/* Set ingress zones */
	for (int i = 0; ingress && ingress[i]; i++) {
		rc = srx_set_str(session, ingress[i], 0, XPATH "/policy[name='%s']/ingress[.='%s']",
				 name, ingress[i]);
		if (rc)
			return rc;
	}

	/* Set egress zones */
	for (int i = 0; egress && egress[i]; i++) {
		rc = srx_set_str(session, egress[i], 0, XPATH "/policy[name='%s']/egress[.='%s']",
				 name, egress[i]);
		if (rc)
			return rc;
	}

	/* Set custom ICMP filters */
	for (int i = 0; icmp_types && icmp_types[i][0]; i++) {
		const char *family = icmp_types[i][0];
		const char *filter = icmp_types[i][1];
		const char *action = icmp_types[i][2];
		const char *type = icmp_types[i][3];

		rc = srx_set_str(session, family, 0,
				 XPATH "/policy[name='%s']/custom/filter[name='%s']/family",
				 name, filter);
		if (rc)
			return rc;

		rc = srx_set_str(session, action, 0,
				 XPATH "/policy[name='%s']/custom/filter[name='%s']/action",
				 name, filter);
		if (rc)
			return rc;

		rc = srx_set_str(session, type, 0,
				 XPATH "/policy[name='%s']/custom/filter[name='%s']/icmp/type",
				 name, filter);
		if (rc)
			return rc;
	}

	return SR_ERR_OK;
}
#endif

int firewall_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *tree, *global;
	struct lyd_node *clist, *cnode;
	sr_error_t err = SR_ERR_OK;
	char **ifaces = NULL;

	if (diff && !lydx_get_xpathf(diff, XPATH))
		return SR_ERR_OK;

	switch (event) {
	case SR_EV_CHANGE:
		/* Generate configuration to /etc/firewalld+ */
		break;

	case SR_EV_ABORT:
		systemf("rm -rf " FIREWALLD_DIR_NEXT);
		return SR_ERR_OK;

	case SR_EV_DONE:
		if (!fisdir(FIREWALLD_DIR_NEXT)) {
			/* Firewall is disabled */
			systemf("initctl -nbq disable firewalld");
			return SR_ERR_OK;
		}

		/* Firewall is enabled, roll in new configuration */
		systemf("rm -rf " FIREWALLD_DIR);
		if (rename(FIREWALLD_DIR_NEXT, FIREWALLD_DIR)) {
			ERRNO("Failed rolling in firewalld configuration");
			return SR_ERR_SYS;
		}

		systemf("initctl -nbq touch firewalld");
		systemf("initctl -nbq enable firewalld");
		return SR_ERR_OK;

	default:
		return SR_ERR_OK;
	}

	tree = config;
	global = lydx_get_descendant(tree, "firewall", NULL);

	/* Clean up any stale /etc/firewalld+ first */
	systemf("rm -rf " FIREWALLD_DIR_NEXT);

	/* If firewall is disabled or not enabled, don't generate config */
	if (!global || !lydx_is_enabled(global, "enabled")) {
		/* Firewall is disabled - no /etc/firewalld+ directory */
		goto done;
	}

	/* Get L3 interfaces for default zone assignment */
	if (interfaces_get_all_l3(tree, &ifaces) != 0) {
		ERROR("Failed to get L3 interfaces");
		ifaces = NULL;
	}

	/* Create /etc/firewalld+ directory structure */
	if (fmkpath(0755, FIREWALLD_DIR_NEXT) ||
	    fmkpath(0755, FIREWALLD_ZONES_DIR) ||
	    fmkpath(0755, FIREWALLD_SERVICES_DIR) ||
	    fmkpath(0755, FIREWALLD_POLICIES_DIR)) {
		ERROR("Failed creating " FIREWALLD_DIR_NEXT " directory structure");
		err = SR_ERR_SYS;
		goto done;
	}

	/* Always generate firewalld.conf when firewall is enabled */
	generate_firewalld_conf(global);

	/*
	 * Regenerate everything if anything in firewall changed, firewalld
	 * handles the 'diff' for us.  Starting priority for policies are at
	 * -14999 because at -15000 is the first "Allow host IPv6" immutable
	 * (default/built-in) policy from firewalld.  We want the user rules
	 * to be between that and the default 'drop-all' implicit rule.
	 */
	if (lydx_get_descendant(diff, "firewall", NULL)) {
		const char *default_zone = lydx_get_cattr(global, "default");
		struct lyd_node *list, *node;
		int priority = -14999;

		/* First, handle explicit deletions by removing files */
		list = lydx_get_descendant(diff, "firewall", "zone", NULL);
		LYX_LIST_FOR_EACH(list, node, "zone") {
			if (lydx_get_op(node) == LYDX_OP_DELETE)
				delete_file(FIREWALLD_ZONES_DIR, lydx_get_cattr(node, "name"));
		}

		list = lydx_get_descendant(diff, "firewall", "service", NULL);
		LYX_LIST_FOR_EACH(list, node, "service") {
			if (lydx_get_op(node) == LYDX_OP_DELETE)
				delete_file(FIREWALLD_SERVICES_DIR, lydx_get_cattr(node, "name"));
		}

		list = lydx_get_descendant(diff, "firewall", "policy", NULL);
		LYX_LIST_FOR_EACH(list, node, "policy") {
			if (lydx_get_op(node) == LYDX_OP_DELETE)
				delete_file(FIREWALLD_POLICIES_DIR, lydx_get_cattr(node, "name"));
		}

		/* Regenerate all non-default zones first */
		clist = lydx_get_descendant(tree, "firewall", "zone", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "zone") {
			const char *name = lydx_get_cattr(cnode, "name");

			/* Skip default zone - we'll do it last */
			if (!strcmp(name, default_zone))
				continue;

			mark_interfaces_used(cnode, ifaces);
			generate_zone(cnode, name, NULL);
		}

		/* Generate default zone last with any unzoned interfaces */
		clist = lydx_get_descendant(tree, "firewall", "zone", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "zone") {
			const char *name = lydx_get_cattr(cnode, "name");

			if (strcmp(name, default_zone))
				continue;

			mark_interfaces_used(cnode, ifaces);
			generate_zone(cnode, name, ifaces);
			break;
		}

		/* Regenerate all services */
		clist = lydx_get_descendant(tree, "firewall", "service", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "service")
			generate_service(cnode, lydx_get_cattr(cnode, "name"));

		/* Regenerate all policies with sequential priority allocation */
		clist = lydx_get_descendant(tree, "firewall", "policy", NULL);
		LYX_LIST_FOR_EACH(clist, cnode, "policy") {
			const char *name = lydx_get_cattr(cnode, "name");

			if (generate_policy(cnode, name, &priority)) {
				ERROR("Failed to generate policy %s", name);
				goto done;
			}
		}
	}

done:
	if (ifaces) {
		for (int i = 0; ifaces[i]; i++)
			free(ifaces[i]);
		free(ifaces);
	}

	return err;
}

static int cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		const char *path, sr_event_t event, unsigned request_id, void *priv)
{
	const char *svc[] = {"ssh", "dhcpv6-client", NULL};
#if INFER_POLICY
	const char *any[] = {"ANY", NULL};
	const char *host[] = {"HOST", NULL};
	const char *icmp_types[][4] = {
		{"ipv6", "na", "accept", "neighbour-advertisement"},
		{"ipv6", "ns", "accept", "neighbour-solicitation"},
		{"ipv6", "ra", "accept", "router-advertisement"},
		{"ipv6", "re", "accept", "redirect"},
		{NULL, NULL, NULL, NULL}
	};
#endif
	size_t cnt = 0;
	int rc;

	if (event != SR_EV_UPDATE && event != SR_EV_CHANGE)
		return 0;

	if (!srx_enabled(session, XPATH "/enabled")) {
		DEBUG("Deleted, or not enabled, not inferring anything.");
		return 0;
	}

	/* If unset, this is the first time we're called */
	if (srx_get_str(session, XPATH "/default"))
		return 0;

	rc = srx_nitems(session, &cnt, XPATH "/zones");
	if (rc == 0 || cnt) {
		WARN("firewall has %zu zone(s) defined, but no default zone! (rc %d)", cnt, rc);
		return 0;
	}

	rc = infer_zone(session, "public", "Public, unknown network. Only SSH and DHCPv6 client allowed.",
			"reject", svc);
	if (rc)
		return rc;

	/* Set up default zone for new networks */
	rc = srx_set_str(session, "public", 0, XPATH "/default");
	if (rc)
		return rc;

#if INFER_POLICY
	/* Infer allow-host-ipv6 policy */
	rc = infer_policy(session, "allow-host-ipv6",
			  "Allows basic IPv6 functionality for the host.",
			  "continue", any, host, icmp_types);
	if (rc)
		return rc;
#endif
	return SR_ERR_OK;
}

static int lockdown(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
		    const sr_val_t *input, const size_t input_cnt, sr_event_t event,
		    uint32_t request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	const char *operation = input->data.string_val;
	int rc;

	DEBUG("lockdown-mode: operation = %s", operation);
	rc = systemf("firewall panic %s", strcmp(operation, "now") ? "off" : "on");
	if (rc) {
		ERROR("lockdown-mode: firewall command failed with exit code %d", rc);
		return SR_ERR_OPERATION_FAILED;
	}

	return SR_ERR_OK;
}

int firewall_rpc_init(struct confd *confd)
{
	int rc;

	REGISTER_RPC(confd->session, XPATH "/lockdown-mode", lockdown, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}


int firewall_candidate_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->cand, MODULE, XPATH "//.", SR_SUBSCR_UPDATE, cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
