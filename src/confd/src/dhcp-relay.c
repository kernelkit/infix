/* SPDX-License-Identifier: BSD-3-Clause */

#include <jansson.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "core.h"
#include "interfaces.h"

#define CFG_XPATH   "/infix-dhcp-relay:dhcp-relay"
#define IFS_XPATH   "/ietf-interfaces:interfaces"
#define RELAY_CONF  "/etc/dhcprelayd.conf"
#define RELAY_NEXT  RELAY_CONF "+"

/* Quoted string, the file format takes \" and \\ */
static void gen_str(FILE *fp, const char *str)
{
	fputc('"', fp);
	for (; *str; str++) {
		if (*str == '"' || *str == '\\')
			fputc('\\', fp);
		fputc(*str, fp);
	}
	fputc('"', fp);
}

static void gen_servers(FILE *fp, struct lyd_node *parent, const char *indent)
{
	struct lyd_node *server;

	LYX_LIST_FOR_EACH(lyd_child(parent), server, "server")
		fprintf(fp, "%sserver %s {\n%s    port = %s\n%s}\n", indent,
			lydx_get_cattr(server, "address"), indent,
			lydx_get_cattr(server, "port"), indent);
}

/*
 * A server on this system answers as 127.0.0.1, which a client cannot
 * renew its lease with, so it must be told the gateway address instead.
 */
static int has_local_server(struct lyd_node *relay)
{
	struct lyd_node *rif, *server;

	LYX_LIST_FOR_EACH(lyd_child(relay), server, "server") {
		if (!strncmp(lydx_get_cattr(server, "address"), "127.", 4))
			return 1;
	}
	LYX_LIST_FOR_EACH(lyd_child(relay), rif, "interface") {
		LYX_LIST_FOR_EACH(lyd_child(rif), server, "server") {
			if (!strncmp(lydx_get_cattr(server, "address"), "127.", 4))
				return 1;
		}
	}

	return 0;
}

/* The str or hex of a circuit-id or remote-id container */
static void gen_id(FILE *fp, struct lyd_node *id, const char *key, const char *indent)
{
	const char *str = lydx_get_cattr(id, "str");
	const char *hex = lydx_get_cattr(id, "hex");

	if (str) {
		fprintf(fp, "%s%s = ", indent, key);
		gen_str(fp, str);
		fputc('\n', fp);
	} else if (hex) {
		fprintf(fp, "%s%s-hex = %s\n", indent, key, hex);
	}
}

static void gen_option82(FILE *fp, struct lyd_node *relay, struct confd *confd)
{
	struct lyd_node *opt82 = lydx_get_child(relay, "option82");
	struct lyd_node *rid;
	const char *type;

	if (!opt82)
		return;

	fprintf(fp, "option82 {\n    policy = %s\n", lydx_get_cattr(opt82, "policy"));

	rid = lydx_get_child(opt82, "remote-id");
	type = lydx_get_cattr(rid, "type");
	if (type && !strcmp(type, "mac")) {
		const char *mac = json_string_value(json_object_get(confd->root, "mac-address"));

		if (mac) {
			fprintf(fp, "    remote-id-hex = %s\n", mac);
		} else {
			WARN("No base MAC address for remote-id, using hostname");
			fprintf(fp, "    remote-id-type = hostname\n");
		}
	} else if (type) {
		fprintf(fp, "    remote-id-type = %s\n", type);
	} else {
		gen_id(fp, rid, "remote-id", "    ");
	}

	fprintf(fp, "}\n");
}

/* Bridge port @name of @rif, with the settings it may have in its port list */
static int gen_port(FILE *fp, struct lyd_node *rif, const char *name, int tagged)
{
	struct lyd_node *port = lydx_find_by_name(rif, "port", name);

	if (port && !lydx_is_enabled(port, "enabled"))
		return 0;

	fprintf(fp, "    port ");
	gen_str(fp, name);
	fprintf(fp, " {\n");
	if (tagged)
		fprintf(fp, "        tagged = true\n");
	if (port && lydx_is_enabled(port, "trusted"))
		fprintf(fp, "        trusted = true\n");
	if (port)
		gen_id(fp, lydx_get_child(port, "circuit-id"), "circuit-id", "        ");
	fprintf(fp, "    }\n");

	return 1;
}

/*
 * The ports of @bridge that requests in VLAN @vid arrive on: untagged
 * on ports with that PVID, tagged on the other members of the VLAN,
 * when there is a VLAN interface to relay them on.  With @vid 0 all
 * ports of the bridge relay.
 */
static int gen_ports(FILE *fp, struct lyd_node *rif, struct lyd_node *ifs,
		     struct lyd_node *bridge, int vid, int vlanif)
{
	const char *brname = lydx_get_cattr(bridge, "name");
	struct lyd_node *cif, *vlan, *member;
	int num = 0;

	LYX_LIST_FOR_EACH(lyd_child(ifs), cif, "interface") {
		struct lyd_node *bp = lydx_get_child(cif, "bridge-port");
		const char *master = lydx_get_cattr(bp, "bridge");

		if (!master || strcmp(master, brname))
			continue;
		if (vid && atoi(lydx_get_cattr(bp, "pvid") ?: "0") != vid)
			continue;

		num += gen_port(fp, rif, lydx_get_cattr(cif, "name"), 0);
	}

	if (!vlanif)
		return num;

	vlan = lydx_get_descendant(lyd_child(bridge), "bridge", "vlans", "vlan", NULL);
	LYX_LIST_FOR_EACH(vlan, vlan, "vlan") {
		if (atoi(lydx_get_cattr(vlan, "vid")) != vid)
			continue;

		LYX_LIST_FOR_EACH(lyd_child(vlan), member, "tagged") {
			const char *name = lyd_get_value(member);
			struct lyd_node *bp;

			/* Listed above when the VLAN is also the PVID of the port */
			bp = lydx_get_child(lydx_find_by_name(ifs, "interface", name), "bridge-port");
			if (!strcmp(name, brname) || atoi(lydx_get_cattr(bp, "pvid") ?: "0") == vid)
				continue;

			num += gen_port(fp, rif, name, 1);
		}
	}

	return num;
}

/* Returns the number of sections written, 0 for a bridge without ports to relay from */
static int gen_interface(FILE *fp, struct lyd_node *rif, struct lyd_node *ifs)
{
	const char *ifname = lydx_get_cattr(rif, "name");
	struct lyd_node *cif = lydx_find_by_name(ifs, "interface", ifname);
	struct lyd_node *bridge = NULL, *vlan;
	int vid = 0, vlanif = 0, num = 1;
	char *ports = NULL;
	size_t len = 0;
	FILE *pfp;

	vlan = lydx_get_child(cif, "vlan");
	if (iftype_from_iface(cif) == IFT_BRIDGE) {
		bridge = cif;
		/* Only untagged requests reach a VLAN filtering bridge itself */
		if (lydx_get_descendant(lyd_child(cif), "bridge", "vlans", NULL))
			vid = atoi(lydx_get_cattr(lydx_get_child(cif, "bridge-port"), "pvid") ?: "0") ?: -1;
	} else if (vlan) {
		bridge = lydx_find_by_name(ifs, "interface", lydx_get_cattr(vlan, "lower-layer-if"));
		if (bridge && iftype_from_iface(bridge) == IFT_BRIDGE) {
			vid = atoi(lydx_get_cattr(vlan, "id"));
			vlanif = 1;
		} else {
			bridge = NULL;
		}
	}

	if (bridge) {
		pfp = open_memstream(&ports, &len);
		if (!pfp)
			return 0;
		num = gen_ports(pfp, rif, ifs, bridge, vid, vlanif);
		fclose(pfp);
	}

	if (!num) {
		NOTE("DHCP relay: no bridge ports to relay from on %s", ifname);
	} else {
		fprintf(fp, "interface ");
		gen_str(fp, ifname);
		fprintf(fp, " {\n");
		if (!lydx_is_enabled(rif, "option82"))
			fprintf(fp, "    option82 = false\n");
		gen_servers(fp, rif, "    ");
		fputs(ports ?: "", fp);
		fprintf(fp, "}\n");
	}
	free(ports);

	return num ? 1 : 0;
}

/* Returns the number of interfaces to relay from, 0 when disabled, -1 on error */
static int gen(struct lyd_node *relay, struct lyd_node *config, struct confd *confd)
{
	struct lyd_node *ifs = lydx_get_descendant(config, "interfaces", NULL);
	struct lyd_node *rif;
	int num = 0;
	FILE *fp;

	if (!lydx_is_enabled(relay, "enabled"))
		return 0;

	fp = fopen(RELAY_NEXT, "w");
	if (!fp) {
		ERRNO("Failed creating %s", RELAY_NEXT);
		return -1;
	}

	gen_servers(fp, relay, "");
	if (lydx_is_enabled(relay, "force-server-identity") || has_local_server(relay))
		fprintf(fp, "force-server-identity = true\n");
	gen_option82(fp, relay, confd);

	LYX_LIST_FOR_EACH(lyd_child(relay), rif, "interface")
		num += gen_interface(fp, rif, ifs);

	fclose(fp);

	return num;
}

static int same(const char *a, const char *b)
{
	return !systemf("cmp -s %s %s", a, b);
}

int dhcp_relay_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
		      sr_event_t event, struct confd *confd)
{
	struct lyd_node *relay = lydx_get_descendant(config, "dhcp-relay", NULL);
	int num;

	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	if (!lydx_get_xpathf(diff, CFG_XPATH)) {
		/* The ports to relay from follow the bridges and their VLANs */
		if (!relay || !lydx_get_xpathf(diff, IFS_XPATH))
			return SR_ERR_OK;
	}

	num = relay ? gen(relay, config, confd) : 0;
	if (num < 0)
		return SR_ERR_SYS;

	if (!num) {
		finit_disable("dhcprelayd");
		erase(RELAY_NEXT);
		erase(RELAY_CONF);
		return SR_ERR_OK;
	}

	/* Most interface changes are not about the ports relayed from */
	if (same(RELAY_NEXT, RELAY_CONF)) {
		erase(RELAY_NEXT);
		return SR_ERR_OK;
	}

	if (rename(RELAY_NEXT, RELAY_CONF)) {
		ERRNO("Failed installing %s", RELAY_CONF);
		return SR_ERR_SYS;
	}

	finit_enable("dhcprelayd");
	finit_reload("dhcprelayd");

	return SR_ERR_OK;
}
