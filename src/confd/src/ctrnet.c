/* SPDX-License-Identifier: BSD-3-Clause */
#include <net/if.h>
#include <srx/common.h>
#include <srx/lyx.h>

#include "core.h"
#include "ctrnet.h"
#include "interfaces.h"

#define NETCONF_DIR  "/run/containers/net"
#define NETCONF_FILE NETCONF_DIR "/%s.json"

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

static bool iface_is_ctrnet(struct lyd_node *node)
{
	return lydx_get_child(node, "container-network") != NULL;
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

static void gen_routes(struct lyd_node *net, FILE *fp)
{
	struct lyd_node *node;
	int first = 1;

	fprintf(fp, "  \"routes\": [");
	LYX_LIST_FOR_EACH(lyd_child(net), node, "route") {
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
 * What ixcont sets up inside the container for an interface it moves
 * there: the addresses of the interface and the container's routes.
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

	fprintf(fp, "{\n  \"addresses\": [");
	gen_addrs(lydx_get_child(cif, "ipv4"), fp, &first);
	gen_addrs(lydx_get_child(cif, "ipv6"), fp, &first);
	fprintf(fp, "%s],\n", first ? "" : "\n  ");

	gen_routes(net, fp);
	fprintf(fp, "\n}\n");

	if (fclose(fp))
		return -errno;

	return 0;
}

int ctrnet_netdag_gen_iface(struct dagger *net, const char *ifname,
			    struct lyd_node *dif, struct lyd_node *cif)
{
	FILE *fp;

	if (iface_is_ctrnet(cif)) {
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
	} else if (iface_is_ctrnet(dif)) {
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
