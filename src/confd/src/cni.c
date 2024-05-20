/* SPDX-License-Identifier: BSD-3-Clause */
#include <net/if.h>
#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include "cni.h"

#define CNI_NAME "/etc/cni/net.d/%s.conflist"

/*
 * When an interface has been handed off to a container it is moved to
 * another network namespace.  This function asks podman for the PID of
 * the container that currently hosts the interface.
 */
pid_t cni_find(const char *ifname)
{
	char buf[32] = { 0 };
	pid_t pid = 0;
	FILE *pp;

	if (fexistf("/sys/class/net/%s", ifname))
		return 0;      /* it's right here, not in a container */

	pp = popenf("r", "container --net %s find", ifname);
	if (!pp)
		return 0;

	if (fgets(buf, sizeof(buf), pp)) {
		chomp(buf);
		pid = atoi(buf);
	}

	pclose(pp);

	return pid;
}

/*
 * This function takes the PID from cni_find() and figures out
 * the name of our ifname inside the container.  For CNI/podman they
 * save the host's name as the interface's ifalias.
 */
static char *cni_find_ifalias(pid_t pid, const char *ifname)
{
	static char buf[IFNAMSIZ + 2];
	char *ptr = NULL;
	FILE *pp;

	pp = popenf("r", "container --net %s find ifname %d", ifname, pid);
	if (!pp)
		return NULL;

	buf[0] = 0;
	if (fgets(buf, sizeof(buf), pp)) {
		chomp(buf);
		ptr = buf;
	}
	pclose(pp);

	return ptr;
}

/*
 * Sometimes we need to perform some kind of 'ip link dev IFNAME'
 * command to find buried interface data.  This function handles
 * the fact that interfaces sometimes are on vacation in another
 * network namespace (container).
 */
FILE *cni_popen(const char *fmt, const char *ifname)
{
	char cmd[strlen(fmt) + 64];
	static char *ifalias;
	pid_t pid;

	pid = cni_find(ifname);
	if (!pid)
		return popenf("re", fmt, ifname);

	ifalias = cni_find_ifalias(pid, ifname);
	if (!ifalias)
		return NULL;

	snprintf(cmd, sizeof(cmd), fmt, ifalias);
	return popenf("re", "nsenter -t %d -n %s", pid, cmd);
}

static bool iface_is_cni(const char *ifname, struct lyd_node *node, const char **type)
{
	struct lyd_node *net = lydx_get_child(node, "container-network");

	if (net) {
		if (type)
			*type = lydx_get_cattr(net, "type");
		return true;
	}

	return false;
}

static void cni_gen_addrs(struct lyd_node *ip, FILE *fp, int *ipam)
{
	struct lyd_node *addr;

	if (!lydx_is_enabled(ip, "enabled"))
		return;

	LYX_LIST_FOR_EACH(lyd_child(ip), addr, "address") {
		struct lyd_node *ip = lydx_get_child(addr, "ip");
		struct lyd_node *len = lydx_get_child(addr, "prefix-length");

		if (*ipam == 0)
			fprintf(fp,
				",\n      \"ipam\": {\n"
				"        \"type\": \"static\",\n"
				"        \"addresses\": [\n");

		fprintf(fp, "%s          { \"address\": \"%s/%s\" }",
			*ipam ? ",\n" : "", lyd_get_value(ip), lyd_get_value(len));
		*ipam = 1;
	}
}

#if 0 /* Unused for now, use container specific global dns and search settings instead. */
static void cni_gen_dns(struct lyd_node *net, FILE *fp, int *first)
{
	struct lyd_node *dns;

	dns = lydx_get_child(net, "dns");
	if (dns) {
		struct lyd_node *node;

		fprintf(fp, ",\n        \"dns\":  {");

		*first = 1;
		LYX_LIST_FOR_EACH(lyd_child(dns), node, "nameservers") {
			if (*first)
				fprintf(fp, "\n          \"nameservers\": [ ");
			else
				fprintf(fp, ", ");

			fprintf(fp, "\"%s\"", lyd_get_value(node));
			(*first)++;
		}
		if (*first > 1)
			fprintf(fp, " ]");

		node = lydx_get_child(dns, "domain");
		if (node) {
			fprintf(fp, "%s\n          \"domain\": \"%s\"",
				*first > 1 ? "," : "", lyd_get_value(node));
			(*first)++;
		}

		LYX_LIST_FOR_EACH(lyd_child(net), node, "search") {
			if (*first)
				fprintf(fp, "%s\n          \"search\": [ ", *first > 1 ? "," : "");
			else
				fprintf(fp, ", ");

			fprintf(fp, "\"%s\"", lyd_get_value(node));
			*first = 0;
		}

		fprintf(fp, "%s\n        }", *first ? "" : "]");
	}
}
#endif

/*
 * Set up IP masquerading bridge which acts as a gateway for nodes behind it.
 * Default subnet, if one is missing in configuration, is: 10.88.0.0/16
 */
static int cni_bridge(struct lyd_node *net, const char *ifname)
{
	struct lyd_node *node;
	int first = 1;
	FILE *fp;

	fp = fopenf("w", CNI_NAME, ifname);
	if (!fp) {
		ERRNO("Failed creating container bridge " CNI_NAME, ifname);
		return -EIO;
	}

	fprintf(fp, "{\n"
		"  \"cniVersion\":    \"1.0.0\",\n"
		"  \"name\":          \"%s\",\n"
		"  \"plugins\": [\n"
		"    {\n"
		"      \"type\":      \"bridge\",\n"
		"      \"bridge\":    \"%s\",\n"
		"      \"isGateway\":   true,\n"
		"      \"ipMasq\":      true,\n"
      		"      \"hairpinMode\": true,\n"
//		"      \"dataDir\":   \"/run/containers/networks\",\n"
		"      \"ipam\": {\n"
		"        \"type\":    \"host-local\"", ifname, ifname);

	LYX_LIST_FOR_EACH(lyd_child(net), node, "route") {
		struct lyd_node *subnet = lydx_get_child(node, "subnet");
		struct lyd_node *gateway = lydx_get_child(node, "gateway");

		if (first)
			fprintf(fp, ",\n        \"routes\": [\n");
		else
			fprintf(fp, ",\n");

		fprintf(fp, "          {\n"
			"            \"dst\": \"%s\"%s\n", lyd_get_value(subnet), gateway ? "," : "");
		if (gateway)
			fprintf(fp, "            \"gw\": \"%s\"\n", lyd_get_value(gateway));
		fprintf(fp, "          }");

		first = 0;
	}
	if (!first)
		fprintf(fp, "        ]");
	else
		fprintf(fp, ",\n        \"routes\": [ { \"dst\": \"0.0.0.0/0\" } ]");

	first = 1;
	LYX_LIST_FOR_EACH(lyd_child(net), node, "subnet") {
		struct lyd_node *subnet = lydx_get_child(node, "subnet");
		struct lyd_node *gateway = lydx_get_child(node, "gateway");

		if (first)
			fprintf(fp, ",\n        \"ranges\": [\n");
		else
			fprintf(fp, ",\n");

		fprintf(fp, "          [{\n"
			"            \"subnet\": \"%s\"%s\n", lyd_get_value(subnet), gateway ? "," : "");
		if (gateway)
			fprintf(fp, "            \"gateway\": \"%s\"\n", lyd_get_value(gateway));
		fprintf(fp, "          }]");

		first = 0;
	}
	if (!first)
		fprintf(fp, "\n        ]");
	else
		/* Default is a customary docker0 local network */
		fprintf(fp, ",\n        \"ranges\": [ [{ \"subnet\": \"172.17.0.0/16\" }] ]");

	fprintf(fp,
		"\n      }\n"	/* /ipam */
		"    },\n"	/* /bridge */
		"    {\n"
		"      \"type\": \"portmap\",\n"
		"      \"capabilities\": {\n"
		"        \"portMappings\": true\n"
		"      }\n"
		"    },\n"	/* /portmap */
		"    {\n"
		"      \"type\": \"firewall\"\n"
		"    },\n"	/* /firewall */
		"    {\n"
		"      \"type\": \"tuning\"\n"
		"    }\n"	/* /tuning */
		"  ]\n"
		"}\n");

	if (fclose(fp))
		return -errno;

	return 0;
}

/*
 * A CNI host interface can be set up without an IP address, but to set
 * a route the IPAM plugin requires setting one or more IP addresses on
 * the ietf-ip level.
 */
static int cni_host(struct lyd_node *net, const char *ifname)
{
	struct lyd_node *node, *ip;
	int addr = 0, route = 0;
	FILE *fp;

	fp = fopenf("w", CNI_NAME, ifname);
	if (!fp) {
		ERRNO("Failed creating container interface " CNI_NAME, ifname);
		return -EIO;
	}

	/*
	 * XXX: currently only support static IP asssignment for
	 *      host-devices.  There is also host-local (see the
	 *      cni_bridge() setup) and dhcp.  The latter support
	 *      running as a daemon (!) and can be useful when we
	 *      add macvlan support.  For more information, see:
	 *      https://www.cni.dev/plugins/current/ipam/
	 */
	fprintf(fp, "{\n"
		"  \"cniVersion\": \"1.0.0\",\n"
		"  \"name\": \"%s\",\n"
		"  \"plugins\": [\n"
		"    {\n"
		"      \"type\": \"host-device\",\n"
		"      \"device\": \"%s\"", ifname, ifname);


	ip = lydx_get_child(lyd_parent(net), "ipv4");
	if (ip)
		cni_gen_addrs(ip, fp, &addr);

	ip = lydx_get_child(lyd_parent(net), "ipv6");
	if (ip)
		cni_gen_addrs(ip, fp, &addr);

	if (addr) {
		fprintf(fp, "\n        ]");

		LYX_LIST_FOR_EACH(lyd_child(net), node, "route") {
			struct lyd_node *subnet = lydx_get_child(node, "subnet");
			struct lyd_node *gateway = lydx_get_child(node, "gateway");

			if (!route) {
				fprintf(fp, ",\n        \"routes\": [\n");
				route++;
			} else
				fprintf(fp, ",\n");

			fprintf(fp, "          {\n"
				"            \"dst\": \"%s\"%s\n", lyd_get_value(subnet), gateway ? "," : "");
			if (gateway)
				fprintf(fp, "            \"gw\": \"%s\"\n", lyd_get_value(gateway));
			fprintf(fp, "          }");
		}
		if (route)
			fprintf(fp, "\n        ]");
	}

	fprintf(fp,
		"%s"
		"    }\n"
		"  ]\n"
		"}\n", (addr || route) ? "\n      }\n" : "");

	if (fclose(fp))
		return -errno;

	return 0;
}

static int iface_gen_cni(const char *ifname, struct lyd_node *cif)
{
	struct lyd_node *net = lydx_get_child(cif, "container-network");
	const char *type = lydx_get_cattr(net, "type");

	/*
	 * klish/sysrepo does not seem to call update callbacks for
	 * presence containers, so we have to be prepared for the
	 * worst here and perform late type inference.  What works:
	 *
	 *     edit container-network               # callback called
	 *
	 * What doesn't work:
	 *
	 *     set container-network                # callback not called
	 *
	 * Funnily enough, "show running-config" shows the empty
	 * "container-network": {}, so someone does their job.
	 */
	if (!type) {
		const char *iftype = lydx_get_cattr(cif, "type");

		if (iftype && !strcmp(iftype, "infix-if-type:bridge"))
			type = "infix-interfaces:bridge";
		else
			type = "infix-interfaces:host";
	}

	if (!strcmp(type, "infix-interfaces:host"))
		return cni_host(net, ifname) ?: 1;

	if (!strcmp(type, "infix-interfaces:bridge"))
		return cni_bridge(net, ifname) ?: 1;

	ERROR("Unknown container network type %s, skipping.", type);
	return 0;
}

int cni_netdag_gen_iface(struct dagger *net, const char *ifname,
			 struct lyd_node *dif, struct lyd_node *cif)
{
	const char *cni_type = NULL;
	FILE *fp;

	if (iface_is_cni(ifname, cif, &cni_type)) {
		int err;

		fp = dagger_fopen_next(net, "init", ifname, 30, "cni.sh");
		if (!fp)
			return -EIO;

		/* Must restart container(s) using this modified network to bite. */
		fprintf(fp, "container -a restart network %s\n", ifname);
		fclose(fp);

		err = iface_gen_cni(ifname, cif);
		if (err)
			return err;
		if (cni_type && !strcmp(cni_type, "bridge"))
			return 1; /* CNI bridges are managed by podman */
	} else if (iface_is_cni(ifname, dif, &cni_type)) {
		/* No longer a container-network, clean up. */
		fp = dagger_fopen_current(net, "exit", ifname, 30, "cni.sh");
		if (!fp)
			return -EIO;

		fprintf(fp, "container -a -f delete network %s >/dev/null\n", ifname);
		fclose(fp);

		if (cni_type && !strcmp(cni_type, "bridge"))
			return 1; /* CNI bridges are managed by podman */
	}

	return 0;
}

int cni_ifchange_cand_infer_type(sr_session_ctx_t *session, const char *path)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	struct lyd_node *node, *net;
	sr_error_t err = SR_ERR_OK;
	char *xpath, *iftype;
	sr_data_t *cfg;

	xpath = xpath_base(path);
	if (!xpath)
		return SR_ERR_SYS;

	err = sr_get_data(session, path, 0, 0, 0, &cfg);
	if (err || !cfg)
		goto err;

	node = lydx_get_descendant(cfg->tree, "interfaces", "interface", NULL);
	if (!node)
		goto out;

	net = lydx_get_child(node, "container-network");
	if (!net)
		goto out;

	if (lydx_get_cattr(net, "type"))
		goto out;	/* CNI type is already set */

	/* Infer from ietf-interface type, reduces typing */
	iftype = srx_get_str(session, "%s/type", xpath);
	if (iftype && !strcmp(iftype, "infix-if-type:bridge"))
		inferred.data.string_val = "bridge";
	else
		inferred.data.string_val = "host";

	err = srx_set_item(session, &inferred, 0, "%s/type", path);
	if (err)
		ERROR("failed setting container-network type %s, err %d: %s",
		      inferred.data.string_val, err, sr_strerror(err));
	if (iftype)
		free(iftype);
out:
	sr_release_data(cfg);
err:
	free(xpath);

	return err;
}
