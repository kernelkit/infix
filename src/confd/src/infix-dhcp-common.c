/* SPDX-License-Identifier: BSD-3-Clause */

#include <ctype.h>
#include <pwd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include <libyang/libyang.h>
#include "core.h"
#include "infix-dhcp-common.h"

int dhcp_option_lookup(const struct lyd_node *id)
{
	const struct lysc_type_enum *enum_type;
	const struct lysc_type_union *uni;
	const struct lysc_node_leaf *leaf;
	const struct lysc_node *schema;
	const struct lysc_type *type;
	LY_ARRAY_COUNT_TYPE u, e;
	const char *name;

	schema = id->schema;
	if (!schema || schema->nodetype != LYS_LEAF)
		return -1;

	leaf = (const struct lysc_node_leaf *)schema;
	type = leaf->type;

	if (type->basetype != LY_TYPE_UNION)
		return -1;	/* We expect a union type */

	uni = (const struct lysc_type_union *)type;
	name = lyd_get_value(id);

	/* Look through each type in the union */
	for (u = 0; u < LY_ARRAY_COUNT(uni->types); u++) {
		type = uni->types[u];

		if (type->basetype == LY_TYPE_ENUM) {
			enum_type = (const struct lysc_type_enum *)type;

			for (e = 0; e < LY_ARRAY_COUNT(enum_type->enums); e++) {
				if (!strcmp(enum_type->enums[e].name, name))
					return enum_type->enums[e].value;
			}
		} else if (type->basetype == LY_TYPE_UINT8) {
			char *endptr;
			long val;

			val = strtol(name, &endptr, 10);
			if (*endptr == 0 && val > 0 && val < 255)
				return (int)val;
		} else if (type->basetype == LY_TYPE_UINT16) {
			char *endptr;
			long val;

			val = strtol(name, &endptr, 10);
			if (*endptr == 0 && val > 0 && val < 65536)
				return (int)val;
		}
	}

	return -1;
}

char *dhcp_hostname(struct lyd_node *cfg, char *str, size_t len)
{
	struct lyd_node *node;
	const char *hostname;
	char *ptr;

	node = lydx_get_xpathf(cfg, "/ietf-system:system/hostname");
	if (!node)
		return NULL;

	hostname = lyd_get_value(node);
	if (!hostname || hostname[0] == 0)
		return NULL;

	ptr = strchr(hostname, '.');
	if (ptr)
		*ptr = 0;

	snprintf(str, len, "-x hostname:%s ", hostname);

	return str;
}

char *dhcp_fqdn(const char *val, char *str, size_t len)
{
	snprintf(str, len, "-F \"%s\" ", val);
	return str;
}

char *dhcp_os_name_version(char *str, size_t len)
{
	char *val;

	if (!str || !len)
		return NULL;

	str[0] = 0;

	val = fgetkey("/etc/os-release", "NAME");
	if (val)
		snprintf(str, len, "-V \"%.32s ", val);

	val = fgetkey("/etc/os-release", "VERSION");
	if (val) {
		strlcat(str, val, len);
		strlcat(str, "\"", len);
	}

	if (strlen(str) > 0 && str[strlen(str) - 1] != '"') {
		str[0] = 0;
		return NULL;
	}

	return str;
}

char *dhcp_compose_option(struct lyd_node *cfg, const char *ifname, struct lyd_node *id,
			  const char *val, const char *hex, char *option, size_t len,
			  char *(*ip_cache_cb)(const char *, char *, size_t))
{
	const char *name = lyd_get_value(id);
	int num = dhcp_option_lookup(id);

	if (num == -1) {
		ERROR("Failed looking up DHCP client %s option %s, skipping.", ifname, name);
		return NULL;
	}

	if (val || hex) {
		switch (num) {
		case 81: /* fqdn */
			if (!val)
				return NULL;
			return dhcp_fqdn(val, option, len);
		case 12: /* hostname */
			if (val && !strcmp(val, "auto"))
				return dhcp_hostname(cfg, option, len);
			/* fallthrough */
		default:
			if (hex) {
				snprintf(option, len, "-x %d:", num);
				strlcat(option, hex, len);
				strlcat(option, " ", len);
			} else {
				/* string value */
				snprintf(option, len, "-x %d:'\"%s\"' ", num, val);
			}
			break;
		}
	} else {
		struct { int num; char *(*cb)(const char *, char *, size_t); } opt[] = {
			{ 50, ip_cache_cb }, /* address */
			{ 81, NULL        }, /* fqdn */
		};

		for (size_t i = 0; i < NELEMS(opt); i++) {
			if (num != opt[i].num)
				continue;

			if (!opt[i].cb || !opt[i].cb(ifname, option, len))
				return NULL;

			return option;
		}

		snprintf(option, len, "-O %d ", num);
	}

	return option;
}

char *dhcp_compose_options(struct lyd_node *cfg, const char *ifname, char **options,
			   struct lyd_node *id, const char *val, const char *hex,
			   char *(*ip_cache_cb)(const char *, char *, size_t))
{
	char opt[300];

	if (!dhcp_compose_option(cfg, ifname, id, val, hex, opt, sizeof(opt), ip_cache_cb))
		return *options;

	if (*options) {
		char *opts;

		opts = realloc(*options, strlen(*options) + strlen(opt) + 1);
		if (!opts) {
			ERROR("failed reallocating options: %s", strerror(errno));
			free(*options);
			return NULL;
		}

		*options = strcat(opts, opt);
	} else
		*options = strdup(opt);

	return *options;
}

/*
 * Default DHCP options for udhcpc, from networking/udhcp/common.c OPTION_REQ
 */
static void infer_options_v4(sr_session_ctx_t *session, const char *xpath)
{
	const char *opt[] = {
		"netmask",
		"broadcast",
		"router",
		"domain",
		"hostname", /* server may use this to register our current name */
		"dns-server",
		"ntp-server" /* will not be activated unless ietf-system also is */
	};
	size_t i;

	for (i = 0; i < NELEMS(opt); i++)
		srx_set_item(session, NULL, 0, "%s/option[id='%s']", xpath, opt[i]);
}

/*
 * Default DHCPv6 options
 * Note: udhcpc6 only supports dns-server (dns) and domain-search (search) with string names.
 * Other options use numeric codes, which dhcp_compose_option() handles automatically.
 */
static void infer_options_v6(sr_session_ctx_t *session, const char *xpath)
{
	const char *opt[] = {
		"dns-server",      /* option 23 - udhcpc6: -O dns */
		"domain-search",   /* option 24 - udhcpc6: -O search */
		"client-fqdn",     /* option 39 - udhcpc6: -O 39 (equivalent to DHCPv4 hostname) */
		"ntp-server"       /* option 56 - udhcpc6: -O 56 */
	};
	size_t i;

	for (i = 0; i < NELEMS(opt); i++)
		srx_set_item(session, NULL, 0, "%s/option[id='%s']", xpath, opt[i]);
}

/*
 * Called from ietf-interfaces.c ifchange_cand() to infer DHCP options
 * for both DHCPv4 and DHCPv6 client configurations
 */
int ifchange_cand_infer_dhcp(sr_session_ctx_t *session, const char *xpath)
{
	sr_error_t err = SR_ERR_OK;
	char *path, *ptr;
	size_t cnt = 0;

	/* Extract path up to and including the dhcp container */
	path = strdup(xpath);
	if (!path)
		return SR_ERR_SYS;

	/* Find the dhcp container in the path */
	ptr = strstr(path, ":dhcp");
	if (!ptr) {
		free(path);
		return SR_ERR_OK;
	}

	/* Move past ":dhcp" to find end of container name */
	ptr += 5; /* strlen(":dhcp") */

	/* If there's more after dhcp (like /arping), truncate it */
	if (*ptr == '/')
		*ptr = '\0';

	/* Check if options already exist */
	err = srx_nitems(session, &cnt, "%s/option", path);
	if (err || cnt) {
		ERROR("%s(): no %s/options err %d cnt %zu", __func__, path, err, cnt);
		goto out;
	}

	/* Infer options based on IPv4 or IPv6 */
	if (strstr(path, ":ipv4/"))
		infer_options_v4(session, path);
	else if (strstr(path, ":ipv6/"))
		infer_options_v6(session, path);

out:
	free(path);
	return err;
}
