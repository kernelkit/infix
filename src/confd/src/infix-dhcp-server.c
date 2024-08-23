/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include <ifaddrs.h>

#define MODULE				"infix-dhcp-server"
#define ROOT_XPATH			"/infix-dhcp-server:"
#define CFG_XPATH			ROOT_XPATH "dhcp-server"

#define DBUS_NAME_DNSMASQ	"uk.org.thekelleys.dnsmasq"

#define DEFAULT_LEASETIME	300		/* seconds */
#define MAX_LEASE_COUNT		1024	/* max. number of leases */
#define MAX_RELAY_SERVER	2 		/* max. number of relay servers */


struct option {
	TAILQ_ENTRY(option) list;
	int					num;
	char 				name[32];
};
TAILQ_HEAD(options, option);

static struct options known_options;


static char * strip(char *line)
{
	char *p;

	while (isspace(*line))
		line++;

	p = line + strlen(line) - 1;
	while (p >= line) {
		if (isspace(*p))
			*(p--) = '\0';
		else
			break;
	}

	return line;
}

static int ip_address_is_valid (const char *ifname, const char *str)
{
	struct ifaddrs *ifaddr, *ifa;
	struct sockaddr_in *sin;
	struct sockaddr_in6 *sin6;
	struct in_addr addr4;
	struct in6_addr addr6;
	int fam, ret = 0;

	if (inet_pton(AF_INET, str, &addr4) == 1)
		fam = AF_INET;
	else if (inet_pton(AF_INET6, str, &addr6) == 1)
		fam = AF_INET6;
	else
		return 0;

	if (getifaddrs(&ifaddr) < 0)
		return 0;
	for (ifa = ifaddr; ifa != NULL; ifa = ifa->ifa_next) {
		if (!ifa->ifa_addr ||
			strcmp(ifa->ifa_name, ifname) != 0 ||
			ifa->ifa_addr->sa_family != fam)
			continue;

		if (fam == AF_INET) {
			sin = (struct sockaddr_in *) ifa->ifa_addr;
			if (sin->sin_addr.s_addr == addr4.s_addr) {
				ret = 1;
				break;
			}
		} else if (fam == AF_INET6) {
			sin6 = (struct sockaddr_in6 *) ifa->ifa_addr;
			if (memcmp(sin6->sin6_addr.s6_addr,
					   addr6.s6_addr, 16) == 0) {
				ret = 1;
				break;
			}
		}
	}
	freeifaddrs(ifaddr);

	return ret;
}

static void add_known_option(char *line)
{
	char *p, *name, *num;
	struct option *opt;

	if (!isdigit(line[0]))
		return;

	p = strchr(line, ' ');
	if (!p)
		return;

	*p = '\0';
	num = line;
	name = p + 1;

	opt = malloc(sizeof(struct option));
	if (opt) {
		opt->num = atoi(strip(num));
		strncpy(opt->name, strip(name), sizeof(opt->name));
		TAILQ_INSERT_TAIL(&known_options, opt, list);
	}
}

static int load_known_options(void)
{
	char line[256];
	FILE *pp;

	TAILQ_INIT(&known_options);

	/*
	 * From dnsmasq.8:
	 *
	 * --help dhcp will display known DHCPv4 configuration options
	 * --help dhcp6 will display DHCPv6 options
	 */
	pp = popen("dnsmasq --help dhcp", "r");
	if (!pp) {
		ERROR("Unable to retrieve known options");
		return -1;
	}
	while (!feof(pp)) {
		if (fgets(line, sizeof(line), pp))
			add_known_option(strip(line));
	}
	pclose(pp);

	return 0;
}

static struct option * get_known_option(const char *name)
{
	struct option *opt;
	int num = -1;

	if (isdigit(name[0]))
		num = atoi(name);

	opt = TAILQ_FIRST(&known_options);
	while (opt) {
		if (num > 0) {
			if (opt->num == num)
				return opt;
		} else {
			if (strcmp(opt->name, name) == 0)
				return opt;
		}
		opt = TAILQ_NEXT(opt, list);
	}

	return NULL;
}

/* configure dnsmasq options */
static int configure_options(FILE *fp, struct lyd_node *cfg)
{
	struct lyd_node *option;
	struct option *opt;
	const char *value, *name;

	LYX_LIST_FOR_EACH(lyd_child(cfg), option, "option") {
		value = lydx_get_cattr(option, "value");
		name  = lydx_get_cattr(option, "name");

		if (!value || !name)
			continue;

		opt = get_known_option(name);
		if (!opt) {
			ERROR("Unknown option %s", name);
			return -1;
		}

		fprintf(fp, "dhcp-option=%d,%s\n", opt->num, value);
	}

	return 0;
}

static int configure_host(FILE *fp, struct lyd_node *host, int leasetime)
{
	struct lyd_node *node;
	const char *ip, *str, *pfx;
	size_t i;
	struct {
		const char *node;
		const char *prefix;
	}  entries[] = {
		{ "mac-address", NULL },
		{ "hostname", NULL },
		{ "client-identifier", "id:" }
	};

	node = lydx_get_child(host, "ip-address");
	if (!node)
		return -1;
	ip = lyd_get_value(node);
	if (!ip || !*ip)
		return -1;

	for (i = 0; i < sizeof(entries)/sizeof(entries[0]); i++) {
		node = lydx_get_child(host, entries[i].node);
		if (!node)
			continue;

		str = lyd_get_value(node);
		if (str && *str) {
			pfx = entries[i].prefix;

			fprintf(fp, "dhcp-host=%s%s,%s,%d\n",
					pfx ? pfx : "",
					ip, str, leasetime);
			return 0;
		}
	}

	return -1;
}

static int configure_pool(FILE *fp, struct lyd_node *pool, int leasetime)
{
	struct lyd_node *node, *host;
	const char *str, *first, *last;

	if ((str = lydx_get_cattr(pool, "pool-name")))
		fprintf(fp, "# pool %s\n", str);

	if ((str = lydx_get_cattr(pool, "lease-time")))
		leasetime = atoi(str);

	if ((node = lydx_get_child(pool, "first-ip-address")))
		first = lyd_get_value(node);
	else
		return -1;

	if ((node = lydx_get_child(pool, "last-ip-address")))
		last = lyd_get_value(node);
	else
		return -1;

	fprintf(fp, "dhcp-range=%s,%s,%d\n", first, last, leasetime);

	LYX_LIST_FOR_EACH(lyd_child(pool), host, "static-allocation") {
		if (configure_host(fp, host, leasetime))
			return -1;
	}

	return 0;
}

static int configure_server(FILE *fp, const char *ifname, struct lyd_node *cfg)
{
	struct lyd_node *pool;
	int leasetime = DEFAULT_LEASETIME;
	const char *str;

	if (configure_options(fp, cfg))
		return -1;

	if ((str = lydx_get_cattr(cfg, "lease-time")))
		leasetime = atoi(str);

	LYX_LIST_FOR_EACH(lyd_child(cfg), pool, "ip-pool") {
		fprintf(fp, "\n");
		if (configure_pool(fp, pool, leasetime))
			return -1;
	}

	return 0;
}

static int configure_relay_info (FILE *fp, struct lyd_node *cfg, const char *id)
{
	struct lyd_node *node;
	const char *str, *p;
	int count = 0;

	/*
	 * Here we configure identifiers for DHCP option 82 (Relay Agent Information)
	 * which get inserted into relayed DHCP requests (see RFC3046).
	 * The DHCP option space is quite limited, thus we only allow 4 bytes for each
	 * identifier.
	 *
	 * If circuit-id is not set, dnsmasq will use the interface index instead.
	 *
	 * (see patches/dnsmasq/2.90/0000-relay-agent-info.patch)
	 */

	node = lydx_get_child(cfg, id);
	if (!node)
		return 0;

	str = lyd_get_value(node);
	for (p = str; p && *p; p++)
		if (*p == ':') count++;

	if (count == 0)
		return 0;

	if (count > 3) {
		ERROR("Overflow in %s (max. 4 bytes allowed)", id);
		return -1;
	}

	fprintf(fp, "dhcp-%s=set:enterprise,%s\n",
			(strcmp(id, "circuit-id") == 0) ? "circuitid" : "remoteid",
			str);

	return 0;
}

static int configure_relay(FILE *fp, const char *ifname, struct lyd_node *cfg)
{
	struct lyd_node *node;
	const char *addr, *str;
	int count = 0;

	if ((node = lydx_get_child(cfg, "address")))
		addr = lyd_get_value(node);

	if (!addr || !*addr) {
		ERROR("Need relay address");
		return -1;
	}
	if (!ip_address_is_valid(ifname, addr)) {
		ERROR("Invalid relay address");
		return -1;
	}

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, "server") {
		if (count >= MAX_RELAY_SERVER) {
			ERROR("Too many relay servers configured");
			return -1;
		}
		if ((str = lydx_get_cattr(node, "ip-address"))) {
			fprintf(fp, "dhcp-relay=%s,%s\n", addr, str);
			count++;
		}
	}

	if (count == 0) {
		ERROR("No relay server configured");
		return -1;
	}

	if (configure_relay_info(fp, cfg, "circuit-id") < 0 ||
		configure_relay_info(fp, cfg, "remote-id") < 0)
		return -1;

	return 0;
}

static int configure_dnsmasq(const char *ifname, struct lyd_node *cfg)
{
	struct lyd_node *node;
	const char *mode = NULL;
	const char *addr;
	FILE *fp = NULL;
	int ret;

	fp = fopenf("w", "/var/run/dnsmasq-%s.conf", ifname);
	if (!fp) {
		ERROR("Failed to create dnsmasq conf for %s: %s", ifname, strerror(errno));
		return -1;
	}

	fprintf(fp, "# Generated by Infix confd\n");
	fprintf(fp, "pid-file=/var/run/dnsmasq-%s.pid\n", ifname);
	fprintf(fp, "enable-dbus=%s.%s\n", DBUS_NAME_DNSMASQ, ifname);
	fprintf(fp, "bind-interfaces\n");
	fprintf(fp, "interface=%s\n", ifname);
	fprintf(fp, "except-interface=lo\n");
	fprintf(fp, "dhcp-leasefile=/var/run/dnsmasq-%s.leases\n", ifname);
	fprintf(fp, "dhcp-lease-max=%d\n", MAX_LEASE_COUNT);

	if (debug)
		fprintf(fp, "log-dhcp\n");

	if ((node = lydx_get_child(cfg, "address"))) {
		addr = lyd_get_value(node);
		if (!ip_address_is_valid(ifname, addr)) {
			ERROR("Invalid server address");
			return -1;
		}
		fprintf(fp, "listen-address=%s\n", addr);
	}

	if (!lydx_is_enabled(cfg, "dns-enabled")) {
		fprintf(fp, "port=0\n");
		fprintf(fp, "no-poll\n");
	}
	fprintf(fp, "\n");

	if ((node = lydx_get_child(cfg, "mode")))
		mode = lyd_get_value(node);

	if (mode && strcmp(mode, "relay") == 0) {
		INFO("Configuring DHCP relay on %s", ifname);
		ret = configure_relay(fp, ifname,
							  lydx_get_child(cfg, "relay"));
	} else {
		INFO("Configuring DHCP server on %s", ifname);
		ret = configure_server(fp, ifname,
							   lydx_get_child(cfg, "server"));
	}

	fclose(fp);

	if (ret != 0)
		fremove("/var/run/dnsmasq-%s.conf", ifname);

	return ret;
}

static int configure_finit(const char *ifname)
{
	FILE *fp;

	fp = fopenf("w", "/etc/finit.d/available/dhcp-server-%s.conf", ifname);
	if (!fp) {
		ERROR("Failed to create finit conf for %s: %s", ifname, strerror(errno));
		return -1;
	}

	fprintf(fp, "# Generated by Infix confd\n");
	fprintf(fp, "service <!> name:dhcp-server :%s <net/%s/running> \\\n"
			"   [2345] dnsmasq -k -u root -C /var/run/dnsmasq-%s.conf \\\n"
			"       -- DHCP server @%s\n",
			ifname, ifname, ifname, ifname);
	fclose(fp);

	return 0;
}

static int configure_dbus(const char *ifname)
{
	FILE *fp;

	if (fexistf("/etc/dbus-1/system.d/dnsmasq-%s.conf", ifname)) {
		return 0;
	}
	fp = fopenf("w", "/etc/dbus-1/system.d/dnsmasq-%s.conf", ifname);
	if (!fp) {
		ERROR("Failed to create dbus conf for %s: %s", ifname, strerror(errno));
		return -1;
	}
	fprintf(fp, "<!DOCTYPE busconfig PUBLIC\n");
	fprintf(fp, " \"-//freedesktop//DTD D-BUS Bus Configuration 1.0//EN\"\n");
	fprintf(fp, " \"http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd\">\n");
	fprintf(fp, "<busconfig>\n");
	fprintf(fp, "  <policy user=\"root\">\n");
	fprintf(fp, "    <allow own=\"%s.%s\"/>\n", DBUS_NAME_DNSMASQ, ifname);
	fprintf(fp, "    <allow send_destination=\"%s.%s\"/>\n", DBUS_NAME_DNSMASQ, ifname);
	fprintf(fp, "  </policy>\n");
	fprintf(fp, "  <policy context=\"default\">\n");
	fprintf(fp, "    <deny own=\"%s.%s\"/>\n", DBUS_NAME_DNSMASQ, ifname);
	fprintf(fp, "    <deny send_destination=\"%s.%s\"/>\n", DBUS_NAME_DNSMASQ, ifname);
	fprintf(fp, "  </policy>\n");
	fprintf(fp, "</busconfig>\n");
	fclose(fp);

	return 0;
}

static void add(const char *ifname, struct lyd_node *cfg)
{
	const char *action = "disable";

	if (configure_dnsmasq(ifname, cfg))
		goto out;

	if (configure_finit(ifname))
		goto out;

	if (configure_dbus(ifname))
		goto out;

	action = "enable";
out:
	systemf("initctl -bfqn %s dhcp-server-%s", action, ifname);
}

static void del(const char *ifname)
{
	systemf("initctl -bfq delete dhcp-server-%s", ifname);
}

static int change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		  const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *global, *diff, *cifs, *difs, *cif, *dif;
	const char *ifname, *cifname;
	sr_error_t err = 0;
	sr_data_t *cfg;
	int enabled = 0;

	switch (event) {
	case SR_EV_DONE:
		break;
	case SR_EV_CHANGE:
	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	err = sr_get_data(session, CFG_XPATH "//.", 0, 0, 0, &cfg);
	if (err || !cfg)
		goto err_abandon;

	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;

	global = lydx_get_descendant(cfg->tree, "dhcp-server", NULL);
	enabled = lydx_is_enabled(global, "enabled");

	cifs = lydx_get_descendant(cfg->tree, "dhcp-server", "server-if", NULL);
	difs = lydx_get_descendant(diff, "dhcp-server", "server-if", NULL);

	/* find the modified one, delete or recreate only that */
	LYX_LIST_FOR_EACH(difs, dif, "server-if") {
		ifname = lydx_get_cattr(dif, "if-name");
		if (!ifname)
			continue;

		if (lydx_get_op(dif) == LYDX_OP_DELETE) {
			del(ifname);
			continue;
		}

		LYX_LIST_FOR_EACH(cifs, cif, "server-if") {
			cifname = lydx_get_cattr(cif, "if-name");
			if (!cifname || strcmp(ifname, cifname))
				continue;

			if (!enabled || !lydx_is_enabled(cif, "enabled"))
				del(ifname);
			else
				add(ifname, cif);
			break;
		}
	}

	if (!enabled) {
		LYX_LIST_FOR_EACH(cifs, cif, "server-if") {
			ifname = lydx_get_cattr(cif, "if-name");
			if (ifname) {
				INFO("DHCP server globally disabled, stopping server on %s", ifname);
				del(ifname);
			}
		}
	}

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:
	return err;
}

static int cleanstats(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
					  const sr_val_t *input, const size_t input_cnt, sr_event_t event,
					  unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	int rc;

	if (input_cnt < 1) {
		ERROR("Not enough input parameters");
		return SR_ERR_SYS;
	}

	rc = systemf("/usr/libexec/statd/dhcp-server-status --clean %s",
				 input[0].data.string_val);

	return (rc == 0) ? SR_ERR_OK : SR_ERR_SYS;
}

int infix_dhcp_server_init(struct confd *confd)
{
	int rc;

	rc = load_known_options();
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, MODULE, CFG_XPATH, 0, change, confd, &confd->sub);
	REGISTER_RPC(confd->session, ROOT_XPATH "clean-statistics", cleanstats, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
