/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <pwd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_module.h>
#include <srx/srx_val.h>

#include "core.h"
#define  ARPING_MSEC    1000
#define  MODULE         "infix-dhcp-client"
#define  XPATH          "/infix-dhcp-client:dhcp-client"
#define  CACHE_TEMPLATE "/var/lib/misc/%s.cache"

static const struct srx_module_requirement reqs[] = {
	{ .dir = YANG_PATH_, .name = MODULE, .rev = "2024-01-30" },
	{ NULL }
};


static char *ip_cache(const char *ifname, char *str, size_t len)
{

	struct in_addr ina;
	char buf[128];
	FILE *fp;

	if (!fexistf(CACHE_TEMPLATE, ifname))
		return NULL;

	fp = fopenf("r", CACHE_TEMPLATE, ifname);
	if (!fp)
		return NULL;

	if (!fgets(buf, sizeof(buf), fp)) {
		fclose(fp);
		return NULL;
	}
	fclose(fp);
	chomp(buf);

	if (!inet_aton(buf, &ina)) {
		erasef(CACHE_TEMPLATE, ifname);
		return NULL;
	}

	snprintf(str, len, "-r %.15s ", buf);

	return str;
}

static char *hostname(const char *ifname, char *str, size_t len)
{
	FILE *fp;
	int pos;

	(void)ifname;

	fp = fopen("/etc/hostname", "r");
	if (!fp)
		return NULL;

	pos = snprintf(str, len, "-x hostname:");
	if (!fgets(&str[pos], len - pos, fp))
		str[0] = 0;
	fclose(fp);
	chomp(str);
	strlcat(str, " ", len);

	return str;
}

static char *fqdn(const char *value, char *str, size_t len)
{
	snprintf(str, len, "-F \"%s\" ", value);
	return str;
}

static char *unquote(char *buf)
{
	char q = buf[0];
	char *ptr;

	if (q != '"' && q != '\'')
		return buf;

	ptr = &buf[strlen(buf) - 1];
	if (*ptr == q) {
		*ptr = 0;
		buf++;
	}

	return buf;
}

static char *os_name_version(char *str, size_t len)
{
	char buf[256];
	FILE *fp;

	if (!str || !len)
		return NULL;

	fp = fopen("/etc/os-release", "r");
	if (!fp)
		return NULL;

	str[0] = 0;
	while (fgets(buf, sizeof(buf), fp)) {
		chomp(buf);
		if (!strncmp(buf, "NAME=", 5))
			snprintf(str, len, "-V \"%.32s ", unquote(&buf[5]));
		if (!strncmp(buf, "VERSION=", 8)) {
			strlcat(str, unquote(&buf[8]), len);
			strlcat(str, "\"", len);
			break;
		}
	}
	fclose(fp);

	if (strlen(str) > 0 && str[strlen(str) - 1] != '"') {
		str[0] = 0;
		return NULL;
	}

	return str;
}

static char *compose_option(const char *ifname, const char *name, const char *value,
			    char *option, size_t len)
{
	if (value) {
		if (isdigit(name[0])) {
			unsigned long opt = strtoul(name, NULL, 0);

			switch (opt) {
			case 81:
				return fqdn(value, option, len);
			default:
				break;
			}

			snprintf(option, len, "-x %s:%s ", name, value);
		} else {
			if (!strcmp(name, "fqdn"))
				fqdn(value, option, len);
			else if (!strcmp(name, "hostname"))
				snprintf(option, len, "-x %s:%s ", name, value);
			else
				snprintf(option, len, "-x %s:'\"%s\"' ", name, value);
		}
	} else {
		struct { char *name; char *(*cb)(const char *, char *, size_t); } opt[] = {
			{ "hostname", hostname },
			{ "address",  ip_cache },
			{ "fqdn",     NULL     },
			{ NULL, NULL }
		};

		for (size_t i = 0; opt[i].name; i++) {
			if (strcmp(name, opt[i].name))
				continue;

			if (!opt[i].cb || !opt[i].cb(ifname, option, len))
				return NULL;

			return option;
		}

		snprintf(option, len, "-O %s ", name);
	}

	return option;
}

static char *compose_options(const char *ifname, char **options, const char *name, const char *value)
{
	char opt[300];

	compose_option(ifname, name, value, opt, sizeof(opt));
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

static char *dhcp_options(const char *ifname, struct lyd_node *cfg)
{
	struct lyd_node *option;
	char *options = NULL;

	LYX_LIST_FOR_EACH(lyd_child(cfg), option, "option") {
		const char *value = lydx_get_cattr(option, "value");
		const char *name  = lydx_get_cattr(option, "name");

		options = compose_options(ifname, &options, name, value);
	}

	if (!options) {
		const char *defaults[] = {
			"router", "dns", "domain", "broadcast", "ntpsrv", "search",
			"address", "staticroutes", "msstaticroutes"
		};

		for (size_t i = 0; i < NELEMS(defaults); i++)
			options = compose_options(ifname, &options, defaults[i], NULL);
	}

	return options ?: strdup("-O subnet -O router -O domain");
}

static void add(const char *ifname, struct lyd_node *cfg)
{
	const char *metric = lydx_get_cattr(cfg, "route-preference");
	const char *client_id = lydx_get_cattr(cfg, "client-id");
	char vendor[128] = { 0 }, do_arp[20] = { 0 };
	char *args = NULL, *options = NULL;
	const char *action = "disable";
	bool arping;
	FILE *fp;

	arping = lydx_is_enabled(cfg, "arping");
	if (arping)
		snprintf(do_arp, sizeof(do_arp), "-a%d", ARPING_MSEC);

	if (client_id && client_id[0]) {
		args = alloca(strlen(client_id) + 12);
		if (args)
			sprintf(args, "-C -x 61:'\"%s\"'", client_id);
	}

	options = dhcp_options(ifname, cfg);
	if (!options) {
		ERROR("failed extracting DHCP options for client %s, aborting!", ifname);
		goto err;
	}

	os_name_version(vendor, sizeof(vendor));

	fp = fopenf("w", "/etc/finit.d/available/dhcp-%s.conf", ifname);
	if (!fp) {
		ERROR("failed creating DHCP client %s: %s", ifname, strerror(errno));
		goto err;
	}

	fprintf(fp, "# Generated by Infix confd\n");
	fprintf(fp, "metric=%s\n", metric);
	fprintf(fp, "service <!> name:dhcp :%s <net/%s/running> \\\n"
		"	[2345] udhcpc -f -p /run/dhcp-%s.pid -t 10 -T 3 -A 10 %s -S -R \\\n"
		"		-o %s \\\n"
		"		-i %s %s %s \\\n"
		"		-- DHCP client @%s\n",
		ifname, ifname, ifname, do_arp,
		options,
		ifname, args ?: "", vendor, ifname);
	fclose(fp);
	action = "enable";
err:
	systemf("initctl -bfqn %s dhcp-%s", action, ifname);
	if (options)
		free(options);
}

static void del(const char *ifname)
{
	systemf("initctl -bfq delete dhcp-%s", ifname);
}

static int change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		  const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *global, *diff, *cifs, *difs, *cif, *dif;
	sr_error_t       err = 0;
	sr_data_t       *cfg;
	int              ena = 0;

	switch (event) {
	case SR_EV_DONE:
		break;
	case SR_EV_CHANGE:
	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	err = sr_get_data(session, XPATH "//.", 0, 0, 0, &cfg);
	if (err)
		goto err_abandon;

	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;

	global = lydx_get_descendant(cfg->tree, "dhcp-client", NULL);
	ena    = lydx_is_enabled(global, "enabled");

	cifs = lydx_get_descendant(cfg->tree, "dhcp-client", "client-if", NULL);
	difs = lydx_get_descendant(diff, "dhcp-client", "client-if", NULL);

	/* find the modified one, delete or recreate only that */
	LYX_LIST_FOR_EACH(difs, dif, "client-if") {
		const char *ifname = lydx_get_cattr(dif, "if-name");

		if (lydx_get_op(dif) == LYDX_OP_DELETE) {
			del(ifname);
			continue;
		}

		LYX_LIST_FOR_EACH(cifs, cif, "client-if") {
			if (strcmp(ifname, lydx_get_cattr(cif, "if-name")))
				continue;

			if (!ena || !lydx_is_enabled(cif, "enabled"))
				del(ifname);
			else
				add(ifname, cif);
			break;
		}
	}

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:

	return err;
}

/*
 * Default DHCP options for udhcpc, from networking/udhcp/common.c OPTION_REQ
 */
static void infer_options(sr_session_ctx_t *session, const char *xpath)
{
	const char *opt[] = {
		"subnet",
		"router",
		"dns",
		"hostname", /* server may use this to register our current name */
		"domain",
		"broadcast",
		"ntpsrv"    /* will not be activated unless ietf-system also is */
	};
	size_t i;

	for (i = 0; i < NELEMS(opt); i++)
		srx_set_item(session, NULL, 0, "%s/option[name='%s']", xpath, opt[i]);
}

static int cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	sr_change_iter_t *iter;
	sr_change_oper_t op;
	sr_val_t *old, *new;
	sr_error_t err;

	switch (event) {
	case SR_EV_UPDATE:
	case SR_EV_CHANGE:
		break;
	default:
		return SR_ERR_OK;
	}

	err = sr_dup_changes_iter(session, XPATH "/client-if//*", &iter);
	if (err)
		return err;

	while (sr_get_change_next(session, iter, &op, &old, &new) == SR_ERR_OK) {
		char xpath[strlen(new->xpath) + 42];
		size_t cnt = 0;
		char *ptr;

		switch (op) {
		case SR_OP_CREATED:
		case SR_OP_MODIFIED:
			break;
		default:
			continue;
		}

		strlcpy(xpath, new->xpath, sizeof(xpath));
		if ((ptr = strstr(xpath, "]/")) == NULL)
			continue;
		ptr[1] = 0;

		err = srx_nitems(session, &cnt, "%s/option", xpath);
		if (err || cnt) {
			continue;
		}

		infer_options(session, xpath);
	}

	sr_free_change_iter(iter);
	return SR_ERR_OK;
}

int infix_dhcp_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, reqs);
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, MODULE, XPATH, 0, change, confd, &confd->sub);
	REGISTER_CHANGE(confd->cand, MODULE, XPATH"//.", SR_SUBSCR_UPDATE, cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
