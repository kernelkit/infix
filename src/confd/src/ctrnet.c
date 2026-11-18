/* SPDX-License-Identifier: BSD-3-Clause */
#include <net/if.h>
#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include "ctrnet.h"
#include "interfaces.h"

#define NETCONF_DIR  "/run/containers/net"
#define NETCONF_FILE NETCONF_DIR "/%s.json"
#define DEFAULT_SUBNET "172.17.0.0/16"

/*
 * When an interface has been handed off to a container it is moved to
 * another network namespace.  This function asks ixcont for the PID of
 * the container that currently hosts the interface.
 */
pid_t ctrnet_find(const char *ifname)
{
	char buf[32] = { 0 };
	pid_t pid = 0;
	FILE *pp;

	if (fexistf("/sys/class/net/%s", ifname))
		return 0;      /* it's right here, not in a container */

	pp = popenf("r", "ixcont --net %s find", ifname);
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
 * The container end of an interface is renamed, the host's name is kept
 * in its ifalias.  Returns the name inside the container at PID.
 */
static char *ctrnet_find_ifalias(pid_t pid, const char *ifname)
{
	static char buf[IFNAMSIZ + 2];
	char *ptr = NULL;
	FILE *pp;

	pp = popenf("r", "ixcont --net %s find ifname %d", ifname, pid);
	if (!pp)
		return NULL;

	buf[0] = 0;
	if (fgets(buf, sizeof(buf), pp)) {
		chomp(buf);
		if (valid_ifname(buf))
			ptr = buf;
		else
			ERROR("Interface %s in container %d has an unsafe name, ignoring", ifname, pid);
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
FILE *ctrnet_popen(const char *fmt, const char *ifname)
{
	char cmd[strlen(fmt) + 64];
	static char *ifalias;
	pid_t pid;

	pid = ctrnet_find(ifname);
	if (!pid)
		return popenf("re", fmt, ifname);

	ifalias = ctrnet_find_ifalias(pid, ifname);
	if (!ifalias)
		return NULL;

	snprintf(cmd, sizeof(cmd), fmt, ifalias);
	return popenf("re", "nsenter -t %d -n %s", pid, cmd);
}

static bool iface_is_ctrnet(struct lyd_node *node, enum iftype *type)
{
	struct lyd_node *net = lydx_get_child(node, "container-network");

	if (net) {
		if (type)
			*type = iftype_from_iface(net);
		return true;
	}

	return false;
}

/*
 * klish/sysrepo does not seem to call update callbacks for presence
 * containers, so the type may be missing: infer it from the interface
 * type, as ctrnet_ifchange_cand_infer_type() does for the CLI.
 */
static const char *ctrnet_type(struct lyd_node *cif)
{
	struct lyd_node *net = lydx_get_child(cif, "container-network");
	const char *type = lydx_get_cattr(net, "type");

	if (type)
		return strstr(type, "bridge") ? "bridge" : "host";

	return iftype_from_iface(cif) == IFT_BRIDGE ? "bridge" : "host";
}

static int gen_addrs(struct lyd_node *ip, FILE *fp, int *first)
{
	struct lyd_node *addr;

	if (!ip || !lydx_is_enabled(ip, "enabled"))
		return 0;

	LYX_LIST_FOR_EACH(lyd_child(ip), addr, "address") {
		fprintf(fp, "%s\n    { \"ip\": \"%s\", \"prefix-length\": %s }", *first ? "" : ",",
			lydx_get_cattr(addr, "ip"), lydx_get_cattr(addr, "prefix-length"));
		*first = 0;
	}

	return 0;
}

static void gen_prefix_list(struct lyd_node *net, FILE *fp, const char *list)
{
	struct lyd_node *node;
	int first = 1;

	fprintf(fp, "  \"%ss\": [", list);
	LYX_LIST_FOR_EACH(lyd_child(net), node, list) {
		const char *gateway = lydx_get_cattr(node, "gateway");

		fprintf(fp, "%s\n    { \"subnet\": \"%s\"", first ? "" : ",", lydx_get_cattr(node, "subnet"));
		if (gateway)
			fprintf(fp, ", \"gateway\": \"%s\"", gateway);
		fprintf(fp, " }");
		first = 0;
	}
	fprintf(fp, "%s]", first ? "" : "\n  ");
}

/*
 * Describe the container network for ixcont: the interface's type and
 * addresses (host type), or the subnets to hand out addresses from
 * (bridge type), and routes to add inside the container.
 */
static int gen_netconf(const char *ifname, struct lyd_node *cif)
{
	struct lyd_node *net = lydx_get_child(cif, "container-network");
	int first = 1;
	FILE *fp;

	fp = fopenfp(0644, NULL, NETCONF_FILE, ifname);
	if (!fp) {
		ERRNO("Failed creating container network " NETCONF_FILE, ifname);
		return -EIO;
	}

	fprintf(fp, "{\n  \"type\": \"%s\",\n", ctrnet_type(cif));

	fprintf(fp, "  \"addresses\": [");
	gen_addrs(lydx_get_child(cif, "ipv4"), fp, &first);
	gen_addrs(lydx_get_child(cif, "ipv6"), fp, &first);
	fprintf(fp, "%s],\n", first ? "" : "\n  ");

	gen_prefix_list(net, fp, "route");
	fprintf(fp, ",\n");
	gen_prefix_list(net, fp, "subnet");
	fprintf(fp, "\n}\n");

	if (fclose(fp))
		return -errno;

	return 0;
}

/*
 * A container bridge is a regular bridge that confd creates, with the
 * gateway address of each subnet and forwarding enabled on it.  ixcont
 * connects containers to it with veth pairs and masquerades in nftables.
 */
static int gen_bridge(struct dagger *net, const char *ifname, struct lyd_node *cif)
{
	struct lyd_node *cnet = lydx_get_child(cif, "container-network");
	struct lyd_node *node;
	int subnets = 0;
	FILE *fp;

	fp = dagger_fopen_net_init(net, ifname, NETDAG_INIT_POST, "ctrnet-bridge.sh");
	if (!fp)
		return -EIO;

	LYX_LIST_FOR_EACH(lyd_child(cnet), node, "subnet") {
		const char *subnet = lydx_get_cattr(node, "subnet");
		const char *gateway = lydx_get_cattr(node, "gateway");
		const char *plen = strchr(subnet, '/');

		if (gateway && plen)
			fprintf(fp, "ip addr replace %s%s dev %s\n", gateway, plen, ifname);
		else
			fprintf(fp, "ip addr replace $(ixcont gateway %s) dev %s\n", subnet, ifname);
		subnets++;
	}
	if (!subnets)
		fprintf(fp, "ip addr replace $(ixcont gateway " DEFAULT_SUBNET ") dev %s\n", ifname);

	fprintf(fp, "sysctl -qw net.ipv4.conf.%s.forwarding=1 net.ipv6.conf.%s.forwarding=1\n",
		ifname, ifname);
	fclose(fp);

	return 0;
}

int ctrnet_netdag_gen_iface(struct dagger *net, const char *ifname,
			    struct lyd_node *dif, struct lyd_node *cif)
{
	enum iftype type;
	FILE *fp;

	if (iface_is_ctrnet(cif, &type)) {
		int err;

		err = gen_netconf(ifname, cif);
		if (err)
			return err;

		/* Containers using this network restart to pick up the change */
		fp = dagger_fopen_net_init(net, ifname, NETDAG_INIT_PRE, "ctrnet.sh");
		if (!fp)
			return -EIO;
		fprintf(fp, "ixcont netchange %s\n", ifname);
		fclose(fp);

		if (!strcmp(ctrnet_type(cif), "bridge"))
			return gen_bridge(net, ifname, cif);
	} else if (iface_is_ctrnet(dif, &type)) {
		/* No longer a container-network, give it back to the host. */
		fp = dagger_fopen_net_exit(net, ifname, NETDAG_EXIT_POST, "ctrnet.sh");
		if (!fp)
			return -EIO;

		fprintf(fp, "ixcont netdel %s\n", ifname);
		fprintf(fp, "rm -f " NETCONF_FILE "\n", ifname);

		/* When both ends of a veth pair are in containers, neither end
		 * is torn down from the host namespace, so the kernel keeps the
		 * pair alive after the interfaces return here.  Have the primary
		 * end delete it, once its container is gone.  When the peer is a
		 * host interface it is the primary and deletes the pair itself
		 * (veth_gen_del); we must not race it here.  Tolerant: the pair
		 * may already be gone.
		 */
		if (lydx_get_child(dif, "veth") && veth_is_primary(dif))
			fprintf(fp, "ip link del dev %s 2>/dev/null || true\n", ifname);

		fclose(fp);
	}

	return 0;
}

int ctrnet_ifchange_cand_infer_type(sr_session_ctx_t *session, const char *path)
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
		goto out;	/* type is already set */

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
