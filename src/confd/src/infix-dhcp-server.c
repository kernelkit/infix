/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include <ifaddrs.h>

#define MODULE			"infix-dhcp-server"
#define ROOT_XPATH		"/infix-dhcp-server:"
#define CFG_XPATH		ROOT_XPATH "dhcp-server"

#define DNSMASQ_GLOBAL_OPTS     "/etc/dnsmasq.d/global-opts.conf"
#define DNSMASQ_SUBNET_FMT      "/etc/dnsmasq.d/%s.conf"
#define DNSMASQ_LEASES          "/var/lib/misc/dnsmasq.leases"
#define DBUS_NAME_DNSMASQ	"uk.org.thekelleys.dnsmasq"

#define DEFAULT_LEASETIME	300  /* seconds */
#define MAX_LEASE_COUNT		1024 /* max. number of leases */
#define MAX_RELAY_SERVER	2    /* max. number of relay servers */


static const char *subnet_tag(const char *subnet)
{
	unsigned int a, b, c, d, m;
	static char tag[32];

	sscanf(subnet, "%u.%u.%u.%u/%u", &a, &b, &c, &d, &m);

	if (m == 8)	   /* Class A */
		snprintf(tag, sizeof(tag), "net-%u", a);
	else if (m == 16)  /* Class B */
		snprintf(tag, sizeof(tag), "net-%u-%u", a, b);
	else if (m == 24)  /* Class C */
		snprintf(tag, sizeof(tag), "net-%u-%u-%u", a, b, c);
	else {
		/* Non-classful, use full format */
		snprintf(tag, sizeof(tag), "net-%u-%u-%u-%u-%u",
			a, b, c, d, m);
	}

	return tag;
}

static const char *host_tag(const char *subnet, const char *addr)
{
	unsigned int a, b, c, d;
	static char tag[128];

	sscanf(addr, "%u.%u.%u.%u", &a, &b, &c, &d);

	if (!subnet)
		snprintf(tag, sizeof(tag), "host-%u-%u-%u-%u", a, b, c, d);
	else
		snprintf(tag, sizeof(tag), "%s-host-%u-%u-%u-%u", subnet, a, b, c, d);

	return tag;
}

static int configure_options(FILE *fp, struct lyd_node *cfg, const char *tag)
{
	struct lyd_node *option;

	LYX_LIST_FOR_EACH(lyd_child(cfg), option, "option") {
		struct lyd_node *id = lydx_get_child(option, "id");
		struct lyd_node *suboption;
		const char *val;
		int num;

		num = dhcp_option_lookup(id);
		if (num == -1) {
			ERROR("Unknown option %s", lyd_get_value(id));
			return -1;
		}

		/*
		 * val is validated by the yang model for each of the various
		 * attributes 'address', 'name', and 'string'.  in the future
		 * we may add 'value' as well, for hexadecimal data
		 */
		val = lydx_get_cattr(option, "name")
			?: lydx_get_cattr(option, "string")
			?: NULL;
		if (!val) {
			val = lydx_get_cattr(option, "address");
			if (val && !strcmp(val, "auto"))
				val = "0.0.0.0";
		}

		if (val) {
			fprintf(fp, "dhcp-option=%s%s%s%d,%s\n",
				tag ? "tag:" : "", tag ?: "",
				tag ? "," : "", num, val);
		} else if ((suboption = lydx_get_descendant(option, "option", "static-route", NULL))) {
			struct lyd_node *net;

			LYX_LIST_FOR_EACH(suboption, net, "static-route") {
				fprintf(fp, "dhcp-option=%s%s%s%d,%s,%s\n",
					tag ? "tag:" : "",
					tag ?: "", tag ? "," : "", num,
					lydx_get_cattr(net, "destination"),
					lydx_get_cattr(net, "next-hop"));
			}
		} else {
			ERROR("Unknown value to option %s", lyd_get_value(id));
			return -1;
		}
	}

	return 0;
}

static const char *host_match(struct lyd_node *match, const char **id)
{
	struct {
		const char *key;
		const char *prefix;
	} choice[] = {
		{ "mac-address", NULL  },
		{ "hostname",    NULL  },
		{ "client-id",   "id:" }
	};

	if (!match)
		return NULL;

	for (size_t i = 0; i < NELEMS(choice); i++) {
		struct lyd_node *node;

		node = lydx_get_child(match, choice[i].key);
		if (!node)
			continue;

		*id = choice[i].prefix;
		return lyd_get_value(node);
	}

	return NULL;
}

/* the address is a key in the host node, so is guaranteed to be set */
static int configure_host(FILE *fp, struct lyd_node *host, const char *subnet)
{
	const char *ip = lydx_get_cattr(host, "address");
	const char *tag = host_tag(NULL, ip);
	const char *match, *id, *name;

	match = host_match(lydx_get_child(host, "match"), &id);
	if (!match)
		return -1;

	fprintf(fp, "\n# Host specific options\n");
	if (configure_options(fp, host, tag))
		return -1;

	name = lydx_get_cattr(host, "hostname");

	/*
	 * set host-specific tag, allow options from that tag,
	 * also allow options from subnet
	 */
	fprintf(fp, "\n# Host %s specific options\n", ip);
	fprintf(fp, "dhcp-host=%s%s,set:%s,set:%s,%s,%s%s%s\n",
		id ? id : "", match, tag, subnet, ip,
		name ?: "", name ? "," : "",
		lydx_get_cattr(host, "lease-time"));

	return 0;
}

static void add(const char *subnet, struct lyd_node *cfg)
{
	const char *tag = subnet_tag(subnet);
	const char *val, *ifname;
	struct lyd_node *node;
	FILE *fp = NULL;
	int rc = 0;

	fp = fopenf("w", DNSMASQ_SUBNET_FMT, tag);
	if (!fp) {
		ERROR("Failed creating dnsmasq conf for %s: %s", subnet, strerror(errno));
		return;
	}

	val = lydx_get_cattr(cfg, "description");
	fprintf(fp, "# Subnet %s%s%s\n", subnet, val ? " - " : "", val ?: "");

	if (debug)
		fprintf(fp, "log-dhcp\n");

	fprintf(fp, "\n# Common options for this subnet\n");
	rc = configure_options(fp, cfg, tag);
	if (rc)
		goto err;

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "host") {
		if ((rc = configure_host(fp, node, tag)))
			goto err;
	}

	/* Optional, may be used to limit scope of subnet */
	ifname = lydx_get_cattr(cfg, "if-name");

	if ((node = lydx_get_child(cfg, "pool"))) {
		const char *start, *end;

		start = lydx_get_cattr(node, "start-address");
		end   = lydx_get_cattr(node, "end-address");

		fprintf(fp, "\n# Subnet pool %s - %s\n", start, end);
		fprintf(fp, "dhcp-range=%s%sset:%s,%s,%s,%s\n",
			ifname ?: "", ifname ? "," : "", tag,
			start, end, lydx_get_cattr(node, "lease-time"));
	}

err:
	fclose(fp);
}

static void del(const char *subnet, struct lyd_node *cfg)
{
	struct in_addr subnet_addr;
	FILE *fp, *next;
	int prefix_len;
	char line[512];

	systemf("initctl -nbq stop dnsmasq");
	fremove(DNSMASQ_SUBNET_FMT, subnet_tag(subnet));

	/* Parse subnet/prefix */
	if (sscanf(subnet, "%15[^/]/%d", line, &prefix_len) != 2)
		goto parse_err;
	if (inet_pton(AF_INET, line, &subnet_addr) != 1) {
	parse_err:
		ERRNO("Failed parsing DHCP server subnet %s for deletion", subnet);
		return;
	}

	fp = fopen(DNSMASQ_LEASES, "r");
	if (!fp)
		return;		/* Nothing to do here */

	/* Create temp file for new leases */
	next = fopen(DNSMASQ_LEASES"+", "w");
	if (!next) {
		ERRNO("Failed creating new leases file %s", DNSMASQ_LEASES"+");
		fclose(fp);
		return;
	}

	/* Copy non-matching leases */
	while (fgets(line, sizeof(line), fp)) {
		char mac[18], ip[16], name[64];
		struct in_addr lease_addr;
		unsigned int lease_time;
		uint32_t subnet_mask;

		if (sscanf(line, "%u %17s %15s %63s", &lease_time, mac, ip, name) < 3)
			continue;

		/* Check if IP is in subnet */
		if (inet_pton(AF_INET, ip, &lease_addr) != 1)
			continue;

		subnet_mask = htonl(~((1UL << (32 - prefix_len)) - 1));
		if ((lease_addr.s_addr & subnet_mask) == (subnet_addr.s_addr & subnet_mask))
			continue;  /* Skip matching lease */

		fputs(line, next);
	}

	fclose(fp);
	fclose(next);

	/* Replace old leases file */
	rename(DNSMASQ_LEASES"+", DNSMASQ_LEASES);
}

static int change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		  const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *global, *diff, *cifs, *difs, *cif, *dif;
	int enabled = 0, added = 0, deleted = 0;
	sr_error_t err = 0;
	sr_data_t *cfg;

	switch (event) {
	case SR_EV_DONE:
		break;
	case SR_EV_CHANGE:
	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	err = sr_get_data(session, "//.", 0, 0, 0, &cfg);
	if (err || !cfg)
		goto err_abandon;

	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;

	global = lydx_get_descendant(cfg->tree, "dhcp-server", NULL);
	enabled = lydx_is_enabled(global, "enabled");

	cifs = lydx_get_descendant(cfg->tree, "dhcp-server", "subnet", NULL);
	difs = lydx_get_descendant(diff, "dhcp-server", "subnet", NULL);

	/* find the modified one, delete or recreate only that */
	LYX_LIST_FOR_EACH(difs, dif, "subnet") {
		const char *subnet = lydx_get_cattr(dif, "subnet");

		if (lydx_get_op(dif) == LYDX_OP_DELETE) {
			del(subnet, dif), deleted++;
			continue;
		}

		LYX_LIST_FOR_EACH(cifs, cif, "subnet") {
			const char *cnet = lydx_get_cattr(cif, "subnet");

			if (strcmp(subnet, cnet))
				continue;

			if (!enabled || !lydx_is_enabled(cif, "enabled"))
				del(subnet, cif), deleted++;
			else
				add(subnet, cif), added++;
			break;
		}
	}

	if (enabled) {
		struct lyd_node *node;
		FILE *fp;

		fp = fopen(DNSMASQ_GLOBAL_OPTS, "w");
		if (!fp)
			goto err_done;

		node = lydx_get_xpathf(cfg->tree, "/ietf-system:system/hostname");
		if (node) {
			const char *hostname = lyd_get_value(node);
			const char *ptr = hostname ? strchr(hostname, '.') : NULL;

			if (ptr)
				fprintf(fp, "domain = %s\n", ++ptr);
		}

		err = configure_options(fp, global, NULL);
		fclose(fp);
		if (err)
			goto err_done;
	} else {
		system("initctl -nbq stop dnsmasq"), deleted++;
		erase(DNSMASQ_LEASES);
		erase(DNSMASQ_GLOBAL_OPTS);

		LYX_LIST_FOR_EACH(cifs, cif, "subnet") {
			const char *subnet = lydx_get_cattr(cif, "subnet");

			INFO("DHCP server globally disabled, stopping server on %s", subnet);
			del(subnet, cif);
		}
	}

err_done:
	if (deleted)
		system("initctl -nbq restart dnsmasq");
	else
		system("initctl -nbq touch dnsmasq");

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:

	return err;
}

static int cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		const char *path, sr_event_t event, unsigned request_id, void *priv)
{
	const char *fmt = "/infix-dhcp-server:dhcp-server/option[id='%s']/address";
	sr_val_t inferred = { .type = SR_STRING_T };
	const char *opt[] = {
		"router",
		"dns-server",
	};
	size_t i, cnt = 0;

	if (event != SR_EV_UPDATE && event != SR_EV_CHANGE)
		return 0;

	if (srx_nitems(session, &cnt, "/infix-dhcp-server:dhcp-server/option") || cnt)
		return 0;

	for (i = 0; i < NELEMS(opt); i++) {
		inferred.data.string_val = "auto";
		srx_set_item(session, &inferred, 0, fmt, opt[i]);
	}

	return 0;
}

static int clear_stats(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
		       const sr_val_t *input, const size_t input_cnt, sr_event_t event,
		       unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	if (systemf("dbus-send --system --dest=uk.org.thekelleys.dnsmasq "
		    "/uk/org/thekelleys/dnsmasq uk.org.thekelleys.dnsmasq.ClearMetrics"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

int infix_dhcp_server_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->session, MODULE, CFG_XPATH, 0, change, confd, &confd->sub);
	REGISTER_CHANGE(confd->cand, MODULE, CFG_XPATH"//.", SR_SUBSCR_UPDATE, cand, confd, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/statistics/clear", clear_stats, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
