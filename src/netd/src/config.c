/* SPDX-License-Identifier: BSD-3-Clause */

#include <dirent.h>
#include <errno.h>
#include <ctype.h>

#include "config.h"
#include "rip.h"

enum config_section {
	SECTION_NONE,
	SECTION_ROUTES,
	SECTION_RIP,
	SECTION_OSPF,  /* Reserved for future use */
};

/*
 * Parse one route line: "ip route PREFIX NEXTHOP [DISTANCE] [tag TAG]"
 *                    or "ipv6 route PREFIX NEXTHOP [DISTANCE] [tag TAG]"
 */
static int parse_route_line(const char *line, struct route_head *head)
{
	char buf[512];
	char *tok, *saveptr, *prefix_str, *nh_str, *dist_str;
	struct route *r;
	char *slash;
	int family;

	snprintf(buf, sizeof(buf), "%s", line);

	/* First token: "ip" or "ipv6" */
	tok = strtok_r(buf, " \t", &saveptr);
	if (!tok)
		return 0;

	if (!strcmp(tok, "ip"))
		family = AF_INET;
	else if (!strcmp(tok, "ipv6"))
		family = AF_INET6;
	else
		return 0; /* skip unknown lines */

	/* Second token: "route" */
	tok = strtok_r(NULL, " \t", &saveptr);
	if (!tok || strcmp(tok, "route"))
		return 0;

	/* Third token: PREFIX/LEN */
	prefix_str = strtok_r(NULL, " \t", &saveptr);
	if (!prefix_str)
		return -1;

	/* Fourth token: NEXTHOP */
	nh_str = strtok_r(NULL, " \t", &saveptr);
	if (!nh_str)
		return -1;

	/* Optional fifth token: DISTANCE */
	dist_str = strtok_r(NULL, " \t\n\r", &saveptr);

	/* Check for optional "tag TAG" */
	tok = strtok_r(NULL, " \t\n\r", &saveptr);
	if (tok && !strcmp(tok, "tag")) {
		/* Skip the tag value - we don't use it yet */
		strtok_r(NULL, " \t\n\r", &saveptr);
	}

	r = calloc(1, sizeof(*r));
	if (!r)
		return -1;

	r->family = family;
	r->distance = dist_str ? (uint8_t)atoi(dist_str) : 1;

	/* Parse prefix */
	slash = strchr(prefix_str, '/');
	if (!slash) {
		free(r);
		return -1;
	}
	*slash = '\0';
	r->prefixlen = (uint8_t)atoi(slash + 1);

	if (family == AF_INET) {
		if (inet_pton(AF_INET, prefix_str, &r->prefix.ip4) != 1) {
			free(r);
			return -1;
		}
	} else {
		if (inet_pton(AF_INET6, prefix_str, &r->prefix.ip6) != 1) {
			free(r);
			return -1;
		}
	}

	/* Parse nexthop */
	if (!strcmp(nh_str, "blackhole")) {
		r->nh_type = NH_BLACKHOLE;
		r->bh_type = BH_DROP;
	} else if (!strcmp(nh_str, "reject")) {
		r->nh_type = NH_BLACKHOLE;
		r->bh_type = BH_REJECT;
	} else if (!strcmp(nh_str, "Null0")) {
		r->nh_type = NH_BLACKHOLE;
		r->bh_type = BH_NULL;
	} else {
		/* Try as IP address first */
		struct in_addr  a4;
		struct in6_addr a6;

		if (family == AF_INET && inet_pton(AF_INET, nh_str, &a4) == 1) {
			r->nh_type = NH_ADDR;
			r->gateway.gw4 = a4;
		} else if (family == AF_INET6 && inet_pton(AF_INET6, nh_str, &a6) == 1) {
			r->nh_type = NH_ADDR;
			r->gateway.gw6 = a6;
		} else {
			/* Treat as interface name */
			r->nh_type = NH_IFNAME;
			snprintf(r->ifname, sizeof(r->ifname), "%s", nh_str);
		}
	}

	TAILQ_INSERT_TAIL(head, r, entries);
	return 0;
}

/*
 * Parse one RIP config line: "network IFNAME"
 *                         or "passive IFNAME"
 *                         or "neighbor ADDRESS"
 *                         or "redistribute TYPE"
 *                         or "timers update N invalid N flush N"
 *                         or "default-metric N"
 *                         or "distance N"
 *                         or "default-route enabled"
 */
static int parse_rip_line(const char *line, struct rip_config *cfg)
{
	char buf[512];
	char *tok, *saveptr;

	snprintf(buf, sizeof(buf), "%s", line);

	tok = strtok_r(buf, " \t", &saveptr);
	if (!tok)
		return 0;

	if (!strcmp(tok, "network")) {
		char *ifname = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!ifname)
			return -1;

		struct rip_network *net = calloc(1, sizeof(*net));
		if (!net)
			return -1;

		snprintf(net->ifname, sizeof(net->ifname), "%s", ifname);
		TAILQ_INSERT_TAIL(&cfg->networks, net, entries);
		cfg->enabled = 1;

	} else if (!strcmp(tok, "passive")) {
		char *ifname = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!ifname)
			return -1;

		/* Find the network and mark it passive */
		struct rip_network *net;
		TAILQ_FOREACH(net, &cfg->networks, entries) {
			if (!strcmp(net->ifname, ifname)) {
				net->passive = 1;
				break;
			}
		}
		/* If not found, create it */
		if (!net) {
			net = calloc(1, sizeof(*net));
			if (!net)
				return -1;
			snprintf(net->ifname, sizeof(net->ifname), "%s", ifname);
			net->passive = 1;
			TAILQ_INSERT_TAIL(&cfg->networks, net, entries);
		}

	} else if (!strcmp(tok, "neighbor")) {
		char *addr_str = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!addr_str)
			return -1;

		struct rip_neighbor *nbr = calloc(1, sizeof(*nbr));
		if (!nbr)
			return -1;

		if (inet_pton(AF_INET, addr_str, &nbr->addr) != 1) {
			free(nbr);
			return -1;
		}

		TAILQ_INSERT_TAIL(&cfg->neighbors, nbr, entries);
		cfg->enabled = 1;

	} else if (!strcmp(tok, "redistribute")) {
		char *type_str = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!type_str)
			return -1;

		struct rip_redistribute *redist = calloc(1, sizeof(*redist));
		if (!redist)
			return -1;

		if (!strcmp(type_str, "connected"))
			redist->type = RIP_REDIST_CONNECTED;
		else if (!strcmp(type_str, "static"))
			redist->type = RIP_REDIST_STATIC;
		else if (!strcmp(type_str, "kernel"))
			redist->type = RIP_REDIST_KERNEL;
		else if (!strcmp(type_str, "ospf"))
			redist->type = RIP_REDIST_OSPF;
		else {
			free(redist);
			return -1;
		}

		TAILQ_INSERT_TAIL(&cfg->redistributes, redist, entries);
		cfg->enabled = 1;

	} else if (!strcmp(tok, "timers")) {
		char *update_tok, *invalid_tok, *flush_tok;

		/* Expect: "timers update N invalid N flush N" */
		tok = strtok_r(NULL, " \t", &saveptr); /* "update" */
		if (!tok || strcmp(tok, "update"))
			return -1;
		update_tok = strtok_r(NULL, " \t", &saveptr);
		if (!update_tok)
			return -1;

		tok = strtok_r(NULL, " \t", &saveptr); /* "invalid" */
		if (!tok || strcmp(tok, "invalid"))
			return -1;
		invalid_tok = strtok_r(NULL, " \t", &saveptr);
		if (!invalid_tok)
			return -1;

		tok = strtok_r(NULL, " \t", &saveptr); /* "flush" */
		if (!tok || strcmp(tok, "flush"))
			return -1;
		flush_tok = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!flush_tok)
			return -1;

		cfg->timers.update = (uint32_t)atoi(update_tok);
		cfg->timers.invalid = (uint32_t)atoi(invalid_tok);
		cfg->timers.flush = (uint32_t)atoi(flush_tok);

	} else if (!strcmp(tok, "default-metric")) {
		char *metric_str = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!metric_str)
			return -1;
		cfg->default_metric = (uint8_t)atoi(metric_str);

	} else if (!strcmp(tok, "distance")) {
		char *dist_str = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!dist_str)
			return -1;
		cfg->distance = (uint8_t)atoi(dist_str);

	} else if (!strcmp(tok, "default-route")) {
		char *enabled_str = strtok_r(NULL, " \t\n\r", &saveptr);
		if (enabled_str && !strcmp(enabled_str, "enabled"))
			cfg->default_route = 1;

	} else if (!strcmp(tok, "debug")) {
		char *debug_type = strtok_r(NULL, " \t\n\r", &saveptr);
		if (!debug_type)
			return -1;

		if (!strcmp(debug_type, "events"))
			cfg->debug_events = 1;
		else if (!strcmp(debug_type, "packet"))
			cfg->debug_packet = 1;
		else if (!strcmp(debug_type, "kernel"))
			cfg->debug_kernel = 1;
		else
			ERROR("Unknown RIP debug type: %s", debug_type);

	} else if (!strcmp(tok, "system")) {
		/* System command to execute after config is applied */
		char *cmd_str = saveptr; /* Rest of the line */
		if (!cmd_str || !*cmd_str) {
			ERROR("Empty system command");
			return -1;
		}

		struct rip_system_cmd *cmd = calloc(1, sizeof(*cmd));
		if (!cmd)
			return -1;

		snprintf(cmd->command, sizeof(cmd->command), "%s", cmd_str);
		TAILQ_INSERT_TAIL(&cfg->system_cmds, cmd, entries);
		cfg->enabled = 1;

	} else {
		/* Unknown RIP command, skip */
		ERROR("Unknown RIP command: %s", tok);
		return 0;
	}

	return 0;
}

static int config_parse_file(const char *path, struct route_head *routes,
			     struct rip_config *rip_cfg)
{
	char line[512];
	enum config_section section = SECTION_NONE;
	FILE *fp;

	fp = fopen(path, "r");
	if (!fp) {
		ERROR("Failed opening %s: %s", path, strerror(errno));
		return -1;
	}

	while (fgets(line, sizeof(line), fp)) {
		char *p = line;

		/* Strip trailing newline/whitespace */
		line[strcspn(line, "\n\r")] = '\0';

		/* Skip leading whitespace */
		while (isspace((unsigned char)*p))
			p++;

		/* Skip empty lines and comments */
		if (*p == '\0' || *p == '!' || *p == '#')
			continue;

		/* Check for section markers */
		if (*p == '[') {
			char *end = strchr(p, ']');
			if (end) {
				*end = '\0';
				p++; /* skip '[' */

				if (!strcmp(p, "routes")) {
					section = SECTION_ROUTES;
					DEBUG("Entered [routes] section");
				} else if (!strcmp(p, "rip")) {
					section = SECTION_RIP;
					DEBUG("Entered [rip] section");
				} else if (!strcmp(p, "ospf")) {
					section = SECTION_OSPF;
					DEBUG("Entered [ospf] section");
				} else {
					section = SECTION_NONE;
					DEBUG("Unknown section: [%s]", p);
				}

				continue;
			}
		}

		/* Parse line based on current section */
		switch (section) {
		case SECTION_ROUTES:
			if (parse_route_line(p, routes))
				ERROR("Failed parsing route in %s: %s", path, line);
			break;

		case SECTION_RIP:
			DEBUG("Parsing RIP line: %s", p);
			if (parse_rip_line(p, rip_cfg))
				ERROR("Failed parsing RIP config in %s: %s", path, line);
			else
				DEBUG("RIP line parsed OK, enabled=%d", rip_cfg->enabled);
			break;

		case SECTION_OSPF:
			/* Reserved for future OSPF support */
			DEBUG("OSPF section not yet implemented");
			break;

		case SECTION_NONE:
			/* For backward compatibility, try parsing as route */
			if (strstr(p, "route"))
				parse_route_line(p, routes);
			break;
		}
	}

	fclose(fp);
	return 0;
}

int config_load(struct route_head *routes, struct rip_config *rip_cfg)
{
	struct dirent **namelist;
	int n, i;

	n = scandir(CONF_DIR, &namelist, NULL, alphasort);
	if (n < 0) {
		if (errno == ENOENT) {
			DEBUG("No config directory %s", CONF_DIR);
			return 0;
		}
		ERROR("scandir %s: %s", CONF_DIR, strerror(errno));
		return -1;
	}

	for (i = 0; i < n; i++) {
		const char *name = namelist[i]->d_name;
		const char *ext;
		char path[PATH_MAX];

		ext = strrchr(name, '.');
		if (!ext || strcmp(ext, ".conf")) {
			free(namelist[i]);
			continue;
		}

		snprintf(path, sizeof(path), "%s/%s", CONF_DIR, name);
		DEBUG("Loading config %s", path);
		config_parse_file(path, routes, rip_cfg);
		free(namelist[i]);
	}

	free(namelist);
	return 0;
}
