/* SPDX-License-Identifier: BSD-3-Clause */

#include <dirent.h>
#include <errno.h>
#include <confuse.h>

#include "config.h"

/*
 * Parse a single route section from libconfuse
 */
static int parse_route_section(cfg_t *cfg_route, struct route_head *head)
{
	const char *prefix_str, *nexthop_str;
	char prefix_copy[128];
	struct in6_addr a6;
	struct in_addr a4;
	struct route *r;
	long distance;
	char *slash;

	prefix_str = cfg_getstr(cfg_route, "prefix");
	nexthop_str = cfg_getstr(cfg_route, "nexthop");
	distance = cfg_getint(cfg_route, "distance");

	if (!prefix_str || !nexthop_str) {
		ERROR("Route missing prefix or nexthop");
		return -1;
	}

	r = calloc(1, sizeof(*r));
	if (!r) {
		ERROR("Failed to allocate route");
		return -1;
	}

	r->distance = (uint8_t)distance;
	r->tag = (uint32_t)cfg_getint(cfg_route, "tag");

	/* Parse prefix - determine address family from presence of ':' */
	snprintf(prefix_copy, sizeof(prefix_copy), "%s", prefix_str);
	slash = strchr(prefix_copy, '/');
	if (!slash) {
		ERROR("Route prefix missing netmask: %s", prefix_str);
		free(r);
		return -1;
	}
	*slash = '\0';
	r->prefixlen = (uint8_t)atoi(slash + 1);

	/* Try IPv6 first (has ':'), then IPv4 */
	if (strchr(prefix_copy, ':')) {
		r->family = AF_INET6;
		if (inet_pton(AF_INET6, prefix_copy, &r->prefix.ip6) != 1) {
			ERROR("Invalid IPv6 prefix: %s", prefix_copy);
			free(r);
			return -1;
		}
	} else {
		r->family = AF_INET;
		if (inet_pton(AF_INET, prefix_copy, &r->prefix.ip4) != 1) {
			ERROR("Invalid IPv4 prefix: %s", prefix_copy);
			free(r);
			return -1;
		}
	}

	/* Parse nexthop - check for special keywords */
	if (!strcmp(nexthop_str, "blackhole")) {
		r->nh_type = NH_BLACKHOLE;
		r->bh_type = BH_DROP;
	} else if (!strcmp(nexthop_str, "reject")) {
		r->nh_type = NH_BLACKHOLE;
		r->bh_type = BH_REJECT;
	} else if (!strcmp(nexthop_str, "Null0")) {
		r->nh_type = NH_BLACKHOLE;
		r->bh_type = BH_NULL;
	} else if (r->family == AF_INET && inet_pton(AF_INET, nexthop_str, &a4) == 1) {
		/* IPv4 address */
		r->nh_type = NH_ADDR;
		r->gateway.gw4 = a4;
	} else if (r->family == AF_INET6 && inet_pton(AF_INET6, nexthop_str, &a6) == 1) {
		/* IPv6 address */
		r->nh_type = NH_ADDR;
		r->gateway.gw6 = a6;
	} else {
		/* Treat as interface name */
		r->nh_type = NH_IFNAME;
		snprintf(r->ifname, sizeof(r->ifname), "%s", nexthop_str);
	}

	TAILQ_INSERT_TAIL(head, r, entries);
	return 0;
}

/*
 * Parse RIP configuration section from libconfuse
 */
static int parse_rip_section(cfg_t *cfg_rip, struct rip_config *rip_cfg)
{
	struct rip_redistribute *redist;
	struct rip_system_cmd *cmd;
	struct rip_neighbor *nbr;
	struct rip_network *net;
	const char *addr_str;
	const char *type_str;
	const char *cmd_str;
	const char *ifname;
	unsigned int n, i;
	cfg_t *cfg_timers;

	/* Check if RIP is enabled */
	if (!cfg_getbool(cfg_rip, "enabled")) {
		DEBUG("RIP is disabled");
		return 0;
	}

	rip_cfg->enabled = 1;

	/* Basic settings */
	rip_cfg->default_metric = (uint8_t)cfg_getint(cfg_rip, "default-metric");
	rip_cfg->distance = (uint8_t)cfg_getint(cfg_rip, "distance");
	rip_cfg->default_route = cfg_getbool(cfg_rip, "default-route");

	/* Debug flags */
	rip_cfg->debug_events = cfg_getbool(cfg_rip, "debug-events");
	rip_cfg->debug_packet = cfg_getbool(cfg_rip, "debug-packet");
	rip_cfg->debug_kernel = cfg_getbool(cfg_rip, "debug-kernel");

	/* Timers subsection */
	cfg_timers = cfg_getsec(cfg_rip, "timers");
	if (cfg_timers) {
		rip_cfg->timers.update = (uint32_t)cfg_getint(cfg_timers, "update");
		rip_cfg->timers.invalid = (uint32_t)cfg_getint(cfg_timers, "invalid");
		rip_cfg->timers.flush = (uint32_t)cfg_getint(cfg_timers, "flush");
	}

	/* Network interfaces */
	n = cfg_size(cfg_rip, "network");
	for (i = 0; i < n; i++) {
		ifname = cfg_getnstr(cfg_rip, "network", i);
		if (!ifname)
			continue;

		net = calloc(1, sizeof(*net));
		if (!net) {
			ERROR("Failed to allocate network");
			continue;
		}

		snprintf(net->ifname, sizeof(net->ifname), "%s", ifname);
		net->passive = 0;
		TAILQ_INSERT_TAIL(&rip_cfg->networks, net, entries);
	}

	/* Passive interfaces - mark existing networks as passive */
	n = cfg_size(cfg_rip, "passive");
	for (i = 0; i < n; i++) {
		int found = 0;

		ifname = cfg_getnstr(cfg_rip, "passive", i);
		if (!ifname)
			continue;

		/* Find the network and mark it passive */
		TAILQ_FOREACH(net, &rip_cfg->networks, entries) {
			if (!strcmp(net->ifname, ifname)) {
				net->passive = 1;
				found = 1;
				break;
			}
		}

		/* If not found, create it as passive */
		if (!found) {
			net = calloc(1, sizeof(*net));
			if (!net) {
				ERROR("Failed to allocate passive network");
				continue;
			}
			snprintf(net->ifname, sizeof(net->ifname), "%s", ifname);
			net->passive = 1;
			TAILQ_INSERT_TAIL(&rip_cfg->networks, net, entries);
		}
	}

	/* Neighbors */
	n = cfg_size(cfg_rip, "neighbor");
	for (i = 0; i < n; i++) {
		addr_str = cfg_getnstr(cfg_rip, "neighbor", i);
		if (!addr_str)
			continue;

		nbr = calloc(1, sizeof(*nbr));
		if (!nbr) {
			ERROR("Failed to allocate neighbor");
			continue;
		}

		if (inet_pton(AF_INET, addr_str, &nbr->addr) != 1) {
			ERROR("Invalid neighbor address: %s", addr_str);
			free(nbr);
			continue;
		}

		TAILQ_INSERT_TAIL(&rip_cfg->neighbors, nbr, entries);
	}

	/* Redistribute routes */
	n = cfg_size(cfg_rip, "redistribute");
	for (i = 0; i < n; i++) {
		type_str = cfg_getnstr(cfg_rip, "redistribute", i);
		if (!type_str)
			continue;

		redist = calloc(1, sizeof(*redist));
		if (!redist) {
			ERROR("Failed to allocate redistribute");
			continue;
		}

		if (!strcmp(type_str, "connected"))
			redist->type = RIP_REDIST_CONNECTED;
		else if (!strcmp(type_str, "static"))
			redist->type = RIP_REDIST_STATIC;
		else if (!strcmp(type_str, "kernel"))
			redist->type = RIP_REDIST_KERNEL;
		else if (!strcmp(type_str, "ospf"))
			redist->type = RIP_REDIST_OSPF;
		else {
			ERROR("Unknown redistribute type: %s", type_str);
			free(redist);
			continue;
		}

		TAILQ_INSERT_TAIL(&rip_cfg->redistributes, redist, entries);
	}

	/* System commands */
	n = cfg_size(cfg_rip, "system");
	for (i = 0; i < n; i++) {
		cmd_str = cfg_getnstr(cfg_rip, "system", i);
		if (!cmd_str || !*cmd_str) {
			ERROR("Empty system command");
			continue;
		}

		cmd = calloc(1, sizeof(*cmd));
		if (!cmd) {
			ERROR("Failed to allocate system command");
			continue;
		}

		snprintf(cmd->command, sizeof(cmd->command), "%s", cmd_str);
		TAILQ_INSERT_TAIL(&rip_cfg->system_cmds, cmd, entries);
	}

	return 0;
}

/*
 * Parse a single config file using libconfuse
 */
static int config_parse_file(const char *path, struct route_head *routes,
			     struct rip_config *rip_cfg)
{
	cfg_opt_t timers_opts[] = {
		CFG_INT("update", 30, CFGF_NONE),
		CFG_INT("invalid", 180, CFGF_NONE),
		CFG_INT("flush", 240, CFGF_NONE),
		CFG_END()
	};

	cfg_opt_t rip_opts[] = {
		CFG_BOOL("enabled", cfg_false, CFGF_NONE),
		CFG_INT("default-metric", 1, CFGF_NONE),
		CFG_INT("distance", 120, CFGF_NONE),
		CFG_BOOL("default-route", cfg_false, CFGF_NONE),
		CFG_BOOL("debug-events", cfg_false, CFGF_NONE),
		CFG_BOOL("debug-packet", cfg_false, CFGF_NONE),
		CFG_BOOL("debug-kernel", cfg_false, CFGF_NONE),
		CFG_STR_LIST("network", NULL, CFGF_NONE),
		CFG_STR_LIST("passive", NULL, CFGF_NONE),
		CFG_STR_LIST("neighbor", NULL, CFGF_NONE),
		CFG_STR_LIST("redistribute", NULL, CFGF_NONE),
		CFG_STR_LIST("system", NULL, CFGF_NONE),
		CFG_SEC("timers", timers_opts, CFGF_NONE),
		CFG_END()
	};

	cfg_opt_t route_opts[] = {
		CFG_STR("prefix", NULL, CFGF_NONE),
		CFG_STR("nexthop", NULL, CFGF_NONE),
		CFG_INT("distance", 1, CFGF_NONE),
		CFG_INT("tag", 0, CFGF_NONE),
		CFG_END()
	};

	cfg_opt_t opts[] = {
		CFG_SEC("route", route_opts, CFGF_MULTI),
		CFG_SEC("rip", rip_opts, CFGF_NONE),
		CFG_END()
	};

	unsigned int i, n;
	cfg_t *cfg_route;
	cfg_t *cfg_rip;
	cfg_t *cfg;
	int ret;

	cfg = cfg_init(opts, CFGF_NONE);
	if (!cfg) {
		ERROR("Failed to initialize libconfuse");
		return -1;
	}

	ret = cfg_parse(cfg, path);
	if (ret == CFG_FILE_ERROR) {
		ERROR("Failed to open config file: %s", path);
		cfg_free(cfg);
		return -1;
	} else if (ret == CFG_PARSE_ERROR) {
		ERROR("Parse error in config file: %s", path);
		cfg_free(cfg);
		return -1;
	}

	/* Parse all route sections */
	n = cfg_size(cfg, "route");
	for (i = 0; i < n; i++) {
		cfg_route = cfg_getnsec(cfg, "route", i);
		if (cfg_route) {
			if (parse_route_section(cfg_route, routes) < 0)
				ERROR("Failed to parse route section %u in %s", i, path);
		}
	}

	/* Parse RIP section if present */
	cfg_rip = cfg_getsec(cfg, "rip");
	if (cfg_rip) {
		if (parse_rip_section(cfg_rip, rip_cfg) < 0)
			ERROR("Failed to parse RIP section in %s", path);
	}

	cfg_free(cfg);
	return 0;
}

int config_load(struct route_head *routes, struct rip_config *rip_cfg)
{
	struct dirent **namelist;
	char path[PATH_MAX];
	const char *name;
	const char *ext;
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
		name = namelist[i]->d_name;

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
