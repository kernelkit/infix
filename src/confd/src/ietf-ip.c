/* SPDX-License-Identifier: BSD-3-Clause */

#include <fnmatch.h>
#include <stdbool.h>
#include <jansson.h>
#include <arpa/inet.h>
#include <net/if.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "ietf-interfaces.h"

int netdag_gen_ipv6_autoconf(struct dagger *net, struct lyd_node *cif,
			     struct lyd_node *dif, FILE *ip)
{
	const char *preferred_lft = "86400", *valid_lft = "604800";
	struct lyd_node *ipconf = lydx_get_child(cif, "ipv6");
	const char *ifname = lydx_get_cattr(dif, "name");
	int global = 0, random = 0;
	struct lyd_node *node;
	FILE *fp;

	if (!ipconf || !lydx_is_enabled(ipconf, "enabled") || is_bridge_port(cif)) {
		fputs(" addrgenmode none", ip);
		return 0;
	}

	node = lydx_get_child(ipconf, "autoconf");
	if (node) {
		global = lydx_is_enabled(node, "create-global-addresses");
		random = lydx_is_enabled(node, "create-temporary-addresses");

		preferred_lft = lydx_get_cattr(node, "temporary-preferred-lifetime");
		valid_lft     = lydx_get_cattr(node, "temporary-valid-lifetime");
	}

	/* 51: must run after interfaces have been created (think: bridge, veth) */
	fp = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "init.sysctl");
	if (fp) {
		/* Autoconfigure addresses using Prefix Information in Router Advertisements */
		fprintf(fp, "net.ipv6.conf.%s.autoconf = %d\n", ifname, global);
		/* The amount of Duplicate Address Detection probes to send. */
		fprintf(fp, "net.ipv6.conf.%s.dad_transmits = %s\n", ifname,
			lydx_get_cattr(ipconf, "dup-addr-detect-transmits"));
		/* Preferred and valid lifetimes for temporary (random) addresses */
		fprintf(fp, "net.ipv6.conf.%s.temp_prefered_lft = %s\n", ifname, preferred_lft);
		fprintf(fp, "net.ipv6.conf.%s.temp_valid_lft = %s\n", ifname, valid_lft);
		fclose(fp);
	}

	fprintf(ip, " addrgenmode %s", random ? "random" : "eui64");

	return 0;
}

/*
 * Check if ipv4 is enabled, only then can autoconf be enabled, in all
 * other cases it must be disabled.  Since we have multiple settings in
 * autoconf, we check if either is modified (diff), in which case we not
 * only enable, but also "touch" the Finit service for avahi-autoipd to
 * ensure it is (re)started.
 *
 * Note: in Infix, regardless of the IPv4 configuration, any link-local
 *       link-local address is disabled when the interface is being used
 *       as a bridge port.
 *
 * Also, IPv4LL is not defined for loopback, so always skip there.
 */
int netdag_gen_ipv4_autoconf(struct dagger *net, struct lyd_node *cif,
			     struct lyd_node *dif)
{
	struct lyd_node *ipconf = lydx_get_child(cif, "ipv4");
	struct lyd_node *ipdiff = lydx_get_child(dif, "ipv4");
	const char *ifname = lydx_get_cattr(dif, "name");
	struct lyd_node *zcip;
	char defaults[64];
	FILE *initctl;
	int err = 0;

	if (!strcmp(ifname, "lo"))
		return 0;

	/* client defults for this interface, needed in both cases */
	snprintf(defaults, sizeof(defaults), "/etc/default/zeroconf-%s", ifname);

	/* no ipv4 at all, ipv4 selectively disabled, or interface is a bridge port */
	if (!ipconf || !lydx_is_enabled(ipconf, "enabled") || is_bridge_port(cif))
		goto disable;

	/*
	 * when enabled, we may have been enabled before, but skipped
	 * for various reasons: was bridge port, ipv4 was disabled...
	 */
	zcip = lydx_get_child(ipconf, "autoconf");
	if (zcip && lydx_is_enabled(zcip, "enabled")) {
		struct lyd_node *node;
		const char *addr;
		int diff = 0;
		FILE *fp;

		/* check for any changes in this container */
		node = lydx_get_child(ipdiff, "autoconf");
		if (node) {
			const struct lyd_node *tmp;

			tmp = lydx_get_child(node, "enabled");
			if (tmp)
				diff++;
			tmp = lydx_get_child(node, "request-address");
			if (tmp)
				diff++;
		}

		fp = fopen(defaults, "w");
		if (!fp) {
			ERRNO("Failed creating %s, cannot enable IPv4LL on %s", defaults, ifname);
			return -EIO;
		}

		fprintf(fp, "ZEROCONF_ARGS=\"--force-bind --syslog ");
		addr = lydx_get_cattr(zcip, "request-address");
		if (addr)
			fprintf(fp, "--start=%s", addr);
		fprintf(fp, "\"\n");
		fclose(fp);

		initctl = dagger_fopen_net_init(net, ifname, NETDAG_INIT_DAEMON, "zeroconf-up.sh");
		if (!initctl)
			return -EIO;

		/* on enable, or reactivation, it is enough to ensure the service is enabled */
		fprintf(initctl, "initctl -bnq enable zeroconf@%s.conf\n", ifname);
		/* on changes to autoconf we must ensure Finit restarts the service */
		if (diff)
			fprintf(initctl, "initctl -bnq touch zeroconf@%s.conf\n", ifname);
	} else {
	disable:
		initctl = dagger_fopen_net_exit(net, ifname, NETDAG_EXIT_DAEMON, "zeroconf-down.sh");
		if (!initctl) {
			/* check if in bootstrap (pre gen 0) */
			if (errno == EUNATCH)
				return 0;
			return -EIO;
		}

		fprintf(initctl, "initctl -bnq disable zeroconf@%s.conf\n", ifname);
		fprintf(initctl, "rm -f %s\n", defaults);
		err = netdag_exit_reload(net);
	}

	fclose(initctl);
	return err;
}


static bool is_std_lo_addr(const char *ifname, const char *ip, const char *pf)
{
	struct in6_addr in6, lo6;
	struct in_addr in4;

	if (strcmp(ifname, "lo"))
		return false;

	if (inet_pton(AF_INET, ip, &in4) == 1)
		return (ntohl(in4.s_addr) == INADDR_LOOPBACK) && !strcmp(pf, "8");

	if (inet_pton(AF_INET6, ip, &in6) == 1) {
		inet_pton(AF_INET6, "::1", &lo6);

		return !memcmp(&in6, &lo6, sizeof(in6))
			&& !strcmp(pf, "128");
	}

	return false;
}

static int netdag_gen_diff_addr(FILE *ip, const char *ifname,
				struct lyd_node *addr)
{
	enum lydx_op op = lydx_get_op(addr);
	struct lyd_node *adr, *pfx;
	struct lydx_diff adrd, pfxd;
	const char *addcmd = "add";

	adr = lydx_get_child(addr, "ip");
	pfx = lydx_get_child(addr, "prefix-length");
	if (!adr || !pfx)
		return -EINVAL;

	lydx_get_diff(adr, &adrd);
	lydx_get_diff(pfx, &pfxd);

	if (op != LYDX_OP_CREATE) {
		fprintf(ip, "address delete %s/%s dev %s\n",
			adrd.old, pfxd.old, ifname);

		if (op == LYDX_OP_DELETE)
			return 0;
	}

	/* When bringing up loopback, the kernel will automatically
	 * add the standard addresses, so don't treat the existance of
	 * these as an error.
	 */
	if ((op == LYDX_OP_CREATE) &&
	    is_std_lo_addr(ifname, adrd.new, pfxd.new))
		addcmd = "replace";

	fprintf(ip, "address %s %s/%s dev %s proto 4\n", addcmd,
		adrd.new, pfxd.new, ifname);
	return 0;
}

static int netdag_gen_diff_addrs(FILE *ip, const char *ifname,
				 struct lyd_node *ipvx)
{
	struct lyd_node *addr;
	int err = 0;

	LYX_LIST_FOR_EACH(lyd_child(ipvx), addr, "address") {
		err = netdag_gen_diff_addr(ip, ifname, addr);
		if (err)
			break;
	}

	return err;
}

static int netdag_set_conf_addrs(FILE *ip, const char *ifname,
				 struct lyd_node *ipvx)
{
	struct lyd_node *addr;

	LYX_LIST_FOR_EACH(lyd_child(ipvx), addr, "address") {
		fprintf(ip, "address add %s/%s dev %s\n",
			lydx_get_cattr(addr, "ip"),
			lydx_get_cattr(addr, "prefix-length"),
			ifname);
	}

	return 0;
}

int netdag_gen_ip_addrs(struct dagger *net, FILE *ip, const char *proto,
			struct lyd_node *cif, struct lyd_node *dif)
{
	struct lyd_node *ipconf = lydx_get_child(cif, proto);
	struct lyd_node *ipdiff = lydx_get_child(dif, proto);
	const char *ifname = lydx_get_cattr(dif, "name");

	if (!ipconf || !lydx_is_enabled(ipconf, "enabled")) {
		if (!cni_find(ifname) && if_nametoindex(ifname)) {
			FILE *fp;

			fp = dagger_fopen_net_exit(net, ifname, NETDAG_EXIT_PRE, "flush.sh");
			if (fp) {
				fprintf(fp, "ip -%c addr flush dev %s\n", proto[3], ifname);
				fclose(fp);
			}
		}
		return 0;
	}

	if (lydx_get_op(lydx_get_child(ipdiff, "enabled")) == LYDX_OP_REPLACE)
		return netdag_set_conf_addrs(ip, ifname, ipconf);

	return netdag_gen_diff_addrs(ip, ifname, ipdiff);
}
