/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * JSON builder for FRR routing configuration using libjansson.
 * Converts internal routing and RIP structures to JSON format
 * following FRR's YANG model for northbound API.
 */

#include <arpa/inet.h>
#include <stdbool.h>
#include <stddef.h>
#include <string.h>

#include <jansson.h>

#include "netd.h"
#include "json_builder.h"

/* Static buffer for JSON output (64KB should be sufficient) */
#define JSON_BUFFER_SIZE (64 * 1024)
static char json_buffer[JSON_BUFFER_SIZE];

/*
 * Helper to check if two routes have the same prefix.
 * Routes with the same prefix should be grouped into one route-list
 * entry with multiple path-list entries.
 */
static bool same_prefix(const struct route *a, const struct route *b)
{
	if (a->family != b->family || a->prefixlen != b->prefixlen)
		return false;

	if (a->family == AF_INET)
		return memcmp(&a->prefix.ip4, &b->prefix.ip4, sizeof(a->prefix.ip4)) == 0;
	else
		return memcmp(&a->prefix.ip6, &b->prefix.ip6, sizeof(a->prefix.ip6)) == 0;
}

/*
 * Build JSON configuration for staticd following FRR's YANG model.
 * Groups routes with the same prefix into a single route-list entry
 * with multiple path-list entries (for ECMP/multipath support).
 */
const char *build_staticd_json(struct route_head *routes)
{
	char prefix_str[INET6_ADDRSTRLEN + 4];
	char addr_buf[INET6_ADDRSTRLEN];
	json_t *control_plane_protocols;
	json_t *control_plane_protocol;
	struct route *r, *curr;
	json_t *frr_staticd;
	json_t *route_entry;
	json_t *route_list;
	json_t *frr_routing;
	const char *afi;
	json_t *nexthop;
	json_t *root;
	char *result;

	root = json_object();
	if (!root)
		return NULL;

	frr_routing = json_object();
	control_plane_protocols = json_object();
	control_plane_protocol = json_array();

	json_t *protocol = json_object();
	json_object_set_new(protocol, "type", json_string("frr-staticd:staticd"));
	json_object_set_new(protocol, "name", json_string("staticd"));
	json_object_set_new(protocol, "vrf", json_string("default"));

	frr_staticd = json_object();
	route_list = json_array();

	r = TAILQ_FIRST(routes);
	while (r != NULL) {
		if (r->family == AF_INET) {
			inet_ntop(AF_INET, &r->prefix.ip4, addr_buf, sizeof(addr_buf));
			afi = "frr-routing:ipv4-unicast";
		} else {
			inet_ntop(AF_INET6, &r->prefix.ip6, addr_buf, sizeof(addr_buf));
			afi = "frr-routing:ipv6-unicast";
		}
		snprintf(prefix_str, sizeof(prefix_str), "%s/%d", addr_buf, (int)r->prefixlen);

		route_entry = json_object();
		json_object_set_new(route_entry, "prefix", json_string(prefix_str));
		json_object_set_new(route_entry, "src-prefix", json_string("::/0"));
		json_object_set_new(route_entry, "afi-safi", json_string(afi));

		json_t *path_list = json_array();

		/* Add all routes with the same prefix as path-list entries */
		curr = r;
		while (curr != NULL && same_prefix(r, curr)) {
			json_t *path = json_object();
			json_object_set_new(path, "table-id", json_integer(0));
			json_object_set_new(path, "distance", json_integer((int)curr->distance));

			json_t *frr_nexthops = json_object();
			json_t *nexthop_array = json_array();
			nexthop = json_object();

			/* Next-hop */
			switch (curr->nh_type) {
			case NH_ADDR:
				if (curr->family == AF_INET)
					inet_ntop(AF_INET, &curr->gateway.gw4, addr_buf, sizeof(addr_buf));
				else
					inet_ntop(AF_INET6, &curr->gateway.gw6, addr_buf, sizeof(addr_buf));

				json_object_set_new(nexthop, "nh-type", json_string(curr->family == AF_INET ? "ip4" : "ip6"));
				json_object_set_new(nexthop, "vrf", json_string("default"));
				json_object_set_new(nexthop, "gateway", json_string(addr_buf));
				json_object_set_new(nexthop, "interface", json_string(""));
				break;

			case NH_IFNAME:
				json_object_set_new(nexthop, "nh-type", json_string("ifindex"));
				json_object_set_new(nexthop, "vrf", json_string("default"));
				json_object_set_new(nexthop, "gateway", json_string(""));
				json_object_set_new(nexthop, "interface", json_string(curr->ifname));
				break;

			case NH_BLACKHOLE:
				json_object_set_new(nexthop, "nh-type", json_string("blackhole"));
				json_object_set_new(nexthop, "vrf", json_string("default"));
				json_object_set_new(nexthop, "gateway", json_string(""));
				json_object_set_new(nexthop, "interface", json_string(""));

				switch (curr->bh_type) {
				case BH_DROP:
					json_object_set_new(nexthop, "bh-type", json_string("null"));
					break;
				case BH_REJECT:
					json_object_set_new(nexthop, "bh-type", json_string("reject"));
					break;
				case BH_NULL:
					json_object_set_new(nexthop, "bh-type", json_string("unspec"));
					break;
				}
				break;
			}

			json_array_append_new(nexthop_array, nexthop);
			json_object_set_new(frr_nexthops, "nexthop", nexthop_array);
			json_object_set_new(path, "frr-nexthops", frr_nexthops);
			json_array_append_new(path_list, path);

			curr = TAILQ_NEXT(curr, entries);
		}

		json_object_set_new(route_entry, "path-list", path_list);
		json_array_append_new(route_list, route_entry);

		r = curr;
	}

	json_object_set_new(frr_staticd, "route-list", route_list);
	json_object_set_new(protocol, "frr-staticd:staticd", frr_staticd);
	json_array_append_new(control_plane_protocol, protocol);
	json_object_set_new(control_plane_protocols, "control-plane-protocol", control_plane_protocol);
	json_object_set_new(frr_routing, "control-plane-protocols", control_plane_protocols);
	json_object_set_new(root, "frr-routing:routing", frr_routing);

	result = json_dumps(root, JSON_COMPACT);
	json_decref(root);

	if (!result)
		return NULL;

	if (strlen(result) >= JSON_BUFFER_SIZE) {
		ERROR("json_builder: buffer overflow (needed %zu, have %d)", strlen(result), JSON_BUFFER_SIZE);
		free(result);
		return NULL;
	}

	strncpy(json_buffer, result, JSON_BUFFER_SIZE - 1);
	json_buffer[JSON_BUFFER_SIZE - 1] = '\0';
	free(result);

	return json_buffer;
}

/*
 * Build JSON configuration for RIP following FRR's YANG model.
 * RIP uses /frr-ripd:ripd/instance path, not control-plane-protocols.
 */
const char *build_rip_json(struct rip_config *rip_cfg)
{
	struct rip_redistribute *redist;
	char addr_buf[INET_ADDRSTRLEN];
	struct rip_neighbor *nbr;
	struct rip_network *net;
	json_t *instance_array;
	const char *proto;
	json_t *instance;
	json_t *ripd_obj;
	json_t *array;
	json_t *root;
	char *result;

	if (!rip_cfg || !rip_cfg->enabled)
		return "";

	root = json_object();
	if (!root)
		return NULL;

	ripd_obj = json_object();
	instance_array = json_array();
	instance = json_object();

	json_object_set_new(instance, "vrf", json_string("default"));

	/* Default metric */
	if (rip_cfg->default_metric != 1)
		json_object_set_new(instance, "default-metric", json_integer((int)rip_cfg->default_metric));

	/* Distance */
	if (rip_cfg->distance != 120) {
		json_t *distance_obj = json_object();
		json_object_set_new(distance_obj, "default", json_integer((int)rip_cfg->distance));
		json_object_set_new(instance, "distance", distance_obj);
	}

	/* Timers */
	if (rip_cfg->timers.update != 30 || rip_cfg->timers.invalid != 180 ||
	    rip_cfg->timers.flush != 240) {
		json_t *timers = json_object();
		json_object_set_new(timers, "update", json_integer(rip_cfg->timers.update));
		json_object_set_new(timers, "timeout", json_integer(rip_cfg->timers.invalid));
		json_object_set_new(timers, "garbage-collection", json_integer(rip_cfg->timers.flush));
		json_object_set_new(instance, "timers", timers);
	}

	/* Default route origination */
	if (rip_cfg->default_route)
		json_object_set_new(instance, "default-information-originate", json_true());

	/* Interfaces (network statements) */
	array = json_array();
	TAILQ_FOREACH(net, &rip_cfg->networks, entries)
		json_array_append_new(array, json_string(net->ifname));
	if (json_array_size(array) > 0)
		json_object_set_new(instance, "interface", array);
	else
		json_decref(array);

	/* Explicit neighbors */
	array = json_array();
	TAILQ_FOREACH(nbr, &rip_cfg->neighbors, entries) {
		inet_ntop(AF_INET, &nbr->addr, addr_buf, sizeof(addr_buf));
		json_array_append_new(array, json_string(addr_buf));
	}
	if (json_array_size(array) > 0)
		json_object_set_new(instance, "explicit-neighbor", array);
	else
		json_decref(array);

	/* Redistribution */
	array = json_array();
	TAILQ_FOREACH(redist, &rip_cfg->redistributes, entries) {
		proto = NULL;
		switch (redist->type) {
		case RIP_REDIST_CONNECTED:
			proto = "connected";
			break;
		case RIP_REDIST_STATIC:
			proto = "static";
			break;
		case RIP_REDIST_KERNEL:
			proto = "kernel";
			break;
		case RIP_REDIST_OSPF:
			proto = "ospf";
			break;
		}

		if (proto) {
			json_t *redist_obj = json_object();
			json_object_set_new(redist_obj, "protocol", json_string(proto));
			json_array_append_new(array, redist_obj);
		}
	}
	if (json_array_size(array) > 0)
		json_object_set_new(instance, "redistribute", array);
	else
		json_decref(array);

	/* Passive interfaces */
	array = json_array();
	TAILQ_FOREACH(net, &rip_cfg->networks, entries) {
		if (net->passive)
			json_array_append_new(array, json_string(net->ifname));
	}
	if (json_array_size(array) > 0)
		json_object_set_new(instance, "passive-interface", array);
	else
		json_decref(array);

	json_array_append_new(instance_array, instance);
	json_object_set_new(ripd_obj, "instance", instance_array);
	json_object_set_new(root, "frr-ripd:ripd", ripd_obj);

	result = json_dumps(root, JSON_COMPACT);
	json_decref(root);

	if (!result)
		return NULL;

	if (strlen(result) >= JSON_BUFFER_SIZE) {
		ERROR("json_builder: buffer overflow (needed %zu, have %d)", strlen(result), JSON_BUFFER_SIZE);
		free(result);
		return NULL;
	}

	strncpy(json_buffer, result, JSON_BUFFER_SIZE - 1);
	json_buffer[JSON_BUFFER_SIZE - 1] = '\0';
	free(result);

	return json_buffer;
}

/*
 * Build complete routing configuration JSON with both static routes and RIP.
 * Static routes go in /frr-routing:routing/control-plane-protocols
 * RIP goes in /frr-ripd:ripd/instance (top level, separate container)
 */
const char *build_routing_json(struct route_head *routes, struct rip_config *rip_cfg)
{
	char prefix_str[INET6_ADDRSTRLEN + 4];
	char addr_buf[INET6_ADDRSTRLEN];
	struct route *r, *curr;
	json_t *root;
	char *result;

	root = json_object();
	if (!root)
		return NULL;

	/* Add static routes */
	if (routes && !TAILQ_EMPTY(routes)) {
		json_t *control_plane_protocols = json_object();
		json_t *control_plane_protocol = json_array();
		json_t *frr_routing = json_object();

		json_t *protocol = json_object();
		json_object_set_new(protocol, "type", json_string("frr-staticd:staticd"));
		json_object_set_new(protocol, "name", json_string("staticd"));
		json_object_set_new(protocol, "vrf", json_string("default"));

		json_t *frr_staticd = json_object();
		json_t *route_list = json_array();

		r = TAILQ_FIRST(routes);
		while (r != NULL) {
			const char *afi;

			if (r->family == AF_INET) {
				inet_ntop(AF_INET, &r->prefix.ip4, addr_buf, sizeof(addr_buf));
				afi = "frr-routing:ipv4-unicast";
			} else {
				inet_ntop(AF_INET6, &r->prefix.ip6, addr_buf, sizeof(addr_buf));
				afi = "frr-routing:ipv6-unicast";
			}
			snprintf(prefix_str, sizeof(prefix_str), "%s/%d", addr_buf, (int)r->prefixlen);

			json_t *route_entry = json_object();
			json_object_set_new(route_entry, "prefix", json_string(prefix_str));
			json_object_set_new(route_entry, "src-prefix", json_string("::/0"));
			json_object_set_new(route_entry, "afi-safi", json_string(afi));

			json_t *path_list = json_array();

			curr = r;
			while (curr != NULL && same_prefix(r, curr)) {
				json_t *path = json_object();
				json_object_set_new(path, "table-id", json_integer(0));
				json_object_set_new(path, "distance", json_integer((int)curr->distance));

				json_t *frr_nexthops = json_object();
				json_t *nexthop_array = json_array();
				json_t *nexthop = json_object();

				switch (curr->nh_type) {
				case NH_ADDR:
					if (curr->family == AF_INET)
						inet_ntop(AF_INET, &curr->gateway.gw4, addr_buf, sizeof(addr_buf));
					else
						inet_ntop(AF_INET6, &curr->gateway.gw6, addr_buf, sizeof(addr_buf));

					json_object_set_new(nexthop, "nh-type", json_string(curr->family == AF_INET ? "ip4" : "ip6"));
					json_object_set_new(nexthop, "vrf", json_string("default"));
					json_object_set_new(nexthop, "gateway", json_string(addr_buf));
					json_object_set_new(nexthop, "interface", json_string(""));
					break;

				case NH_IFNAME:
					json_object_set_new(nexthop, "nh-type", json_string("ifindex"));
					json_object_set_new(nexthop, "vrf", json_string("default"));
					json_object_set_new(nexthop, "gateway", json_string(""));
					json_object_set_new(nexthop, "interface", json_string(curr->ifname));
					break;

				case NH_BLACKHOLE:
					json_object_set_new(nexthop, "nh-type", json_string("blackhole"));
					json_object_set_new(nexthop, "vrf", json_string("default"));
					json_object_set_new(nexthop, "gateway", json_string(""));
					json_object_set_new(nexthop, "interface", json_string(""));

					switch (curr->bh_type) {
					case BH_DROP:
						json_object_set_new(nexthop, "bh-type", json_string("null"));
						break;
					case BH_REJECT:
						json_object_set_new(nexthop, "bh-type", json_string("reject"));
						break;
					case BH_NULL:
						json_object_set_new(nexthop, "bh-type", json_string("unspec"));
						break;
					}
					break;
				}

				json_array_append_new(nexthop_array, nexthop);
				json_object_set_new(frr_nexthops, "nexthop", nexthop_array);
				json_object_set_new(path, "frr-nexthops", frr_nexthops);
				json_array_append_new(path_list, path);

				curr = TAILQ_NEXT(curr, entries);
			}

			json_object_set_new(route_entry, "path-list", path_list);
			json_array_append_new(route_list, route_entry);

			r = curr;
		}

		json_object_set_new(frr_staticd, "route-list", route_list);
		json_object_set_new(protocol, "frr-staticd:staticd", frr_staticd);
		json_array_append_new(control_plane_protocol, protocol);
		json_object_set_new(control_plane_protocols, "control-plane-protocol", control_plane_protocol);
		json_object_set_new(frr_routing, "control-plane-protocols", control_plane_protocols);
		json_object_set_new(root, "frr-routing:routing", frr_routing);
	}

	/* Add RIP config (separate top-level container) */
	if (rip_cfg && rip_cfg->enabled) {
		struct rip_redistribute *redist;
		struct rip_neighbor *nbr;
		struct rip_network *net;
		const char *proto;
		json_t *array;

		json_t *ripd_obj = json_object();
		json_t *instance_array = json_array();
		json_t *instance = json_object();

		json_object_set_new(instance, "vrf", json_string("default"));

		if (rip_cfg->default_metric != 1)
			json_object_set_new(instance, "default-metric", json_integer((int)rip_cfg->default_metric));

		if (rip_cfg->distance != 120) {
			json_t *distance_obj = json_object();
			json_object_set_new(distance_obj, "default", json_integer((int)rip_cfg->distance));
			json_object_set_new(instance, "distance", distance_obj);
		}

		if (rip_cfg->timers.update != 30 || rip_cfg->timers.invalid != 180 ||
		    rip_cfg->timers.flush != 240) {
			json_t *timers = json_object();
			json_object_set_new(timers, "update", json_integer(rip_cfg->timers.update));
			json_object_set_new(timers, "timeout", json_integer(rip_cfg->timers.invalid));
			json_object_set_new(timers, "garbage-collection", json_integer(rip_cfg->timers.flush));
			json_object_set_new(instance, "timers", timers);
		}

		if (rip_cfg->default_route)
			json_object_set_new(instance, "default-information-originate", json_true());

		array = json_array();
		TAILQ_FOREACH(net, &rip_cfg->networks, entries)
			json_array_append_new(array, json_string(net->ifname));
		if (json_array_size(array) > 0)
			json_object_set_new(instance, "interface", array);
		else
			json_decref(array);

		array = json_array();
		TAILQ_FOREACH(nbr, &rip_cfg->neighbors, entries) {
			inet_ntop(AF_INET, &nbr->addr, addr_buf, sizeof(addr_buf));
			json_array_append_new(array, json_string(addr_buf));
		}
		if (json_array_size(array) > 0)
			json_object_set_new(instance, "explicit-neighbor", array);
		else
			json_decref(array);

		array = json_array();
		TAILQ_FOREACH(redist, &rip_cfg->redistributes, entries) {
			proto = NULL;
			switch (redist->type) {
			case RIP_REDIST_CONNECTED:
				proto = "connected";
				break;
			case RIP_REDIST_STATIC:
				proto = "static";
				break;
			case RIP_REDIST_KERNEL:
				proto = "kernel";
				break;
			case RIP_REDIST_OSPF:
				proto = "ospf";
				break;
			}

			if (proto) {
				json_t *redist_obj = json_object();
				json_object_set_new(redist_obj, "protocol", json_string(proto));
				json_array_append_new(array, redist_obj);
			}
		}
		if (json_array_size(array) > 0)
			json_object_set_new(instance, "redistribute", array);
		else
			json_decref(array);

		array = json_array();
		TAILQ_FOREACH(net, &rip_cfg->networks, entries) {
			if (net->passive)
				json_array_append_new(array, json_string(net->ifname));
		}
		if (json_array_size(array) > 0)
			json_object_set_new(instance, "passive-interface", array);
		else
			json_decref(array);

		json_array_append_new(instance_array, instance);
		json_object_set_new(ripd_obj, "instance", instance_array);
		json_object_set_new(root, "frr-ripd:ripd", ripd_obj);
	}

	result = json_dumps(root, JSON_COMPACT);
	json_decref(root);

	if (!result)
		return NULL;

	if (strlen(result) >= JSON_BUFFER_SIZE) {
		ERROR("json_builder: buffer overflow (needed %zu, have %d)", strlen(result), JSON_BUFFER_SIZE);
		free(result);
		return NULL;
	}

	strncpy(json_buffer, result, JSON_BUFFER_SIZE - 1);
	json_buffer[JSON_BUFFER_SIZE - 1] = '\0';
	free(result);

	return json_buffer;
}
