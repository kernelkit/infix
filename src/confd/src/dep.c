/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdarg.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "interfaces.h"
#include "core.h"

#define DEP_MAX_PASSES 10

#define IFACE "/ietf-interfaces:interfaces/interface"
#define RADIO "/ietf-hardware:hardware/component[name='%s']/infix-hardware:wifi-radio"

struct dep {
	struct lyd_node **diff;
	struct lyd_node  *config;
	int               added;
	int               err;
};

/* Add a node to the diff, unless it is already there */
static void dep_add(struct dep *d, const char *value, const char *fmt, ...)
	__attribute__ ((format (printf, 3, 4)));
static void dep_add(struct dep *d, const char *value, const char *fmt, ...)
{
	struct lyd_node *node = NULL, *root;
	char xpath[256];
	va_list ap;
	int rc;

	if (d->err)
		return;

	va_start(ap, fmt);
	rc = vsnprintf(xpath, sizeof(xpath), fmt, ap);
	va_end(ap);
	if (rc < 0 || (size_t)rc >= sizeof(xpath)) {
		ERROR("Dependency path too long: %s", xpath);
		goto err;
	}

	if (lydx_get_xpathf(*d->diff, "%s", xpath))
		return;

	rc = lyd_new_path(NULL, LYD_CTX(*d->diff), xpath, value, LYD_NEW_PATH_UPDATE, &node);
	if (rc != LY_SUCCESS || !node) {
		ERROR("Failed creating %s, rc=%d", xpath, rc);
		goto err;
	}

	root = node;
	while (root->parent)
		root = lyd_parent(root);

	rc = lyd_merge_siblings(d->diff, root, LYD_MERGE_DESTRUCT);
	if (rc != LY_SUCCESS) {
		ERROR("Failed merging %s into diff, rc=%d", xpath, rc);
		lyd_free_tree(root);
		goto err;
	}

	node = lydx_get_xpathf(*d->diff, "%s", xpath);
	if (!node) {
		ERROR("Failed adding %s to diff", xpath);
		goto err;
	}
	lyd_new_meta(LYD_CTX(node), node, NULL, "yang:operation", "replace", false, NULL);

	DEBUG("Added %s to diff", xpath);
	d->added++;
	return;
err:
	d->err = 1;
}

/* Add sub of every configured interface that matches the predicate */
static void dep_add_ifaces(struct dep *d, const char *value, const char *sub, const char *fmt, ...)
	__attribute__ ((format (printf, 4, 5)));
static void dep_add_ifaces(struct dep *d, const char *value, const char *sub, const char *fmt, ...)
{
	struct ly_set *ifaces;
	char pred[192];
	va_list ap;
	uint32_t i;

	va_start(ap, fmt);
	vsnprintf(pred, sizeof(pred), fmt, ap);
	va_end(ap);

	ifaces = lydx_find_xpathf(d->config, IFACE "[%s]", pred);
	for (i = 0; ifaces && i < ifaces->count; i++)
		dep_add(d, value, IFACE "[name='%s']/%s", lydx_get_cattr(ifaces->dnodes[i], "name"), sub);
	ly_set_free(ifaces, NULL);
}

static void dep_symmetric_keys(struct dep *d)
{
	struct lyd_node *keys, *key;

	keys = lydx_get_descendant(*d->diff, "keystore", "symmetric-keys", "symmetric-key", NULL);
	LYX_LIST_FOR_EACH(keys, key, "symmetric-key") {
		const char *name = lydx_get_cattr(key, "name");

		dep_add_ifaces(d, name, "infix-interfaces:wifi/secret",
			       "infix-interfaces:wifi/secret='%s'", name);
		dep_add_ifaces(d, NULL, "infix-interfaces:wireguard",
			       "infix-interfaces:wireguard/peers/preshared-key='%s'", name);
		dep_add_ifaces(d, NULL, "infix-interfaces:wireguard",
			       "infix-interfaces:wireguard/peers/peer/preshared-key='%s'", name);
		dep_add_ifaces(d, name, "infix-interfaces:ppp/secret",
			       "infix-interfaces:ppp/secret='%s'", name);
	}
}

static void dep_asymmetric_keys(struct dep *d)
{
	struct lyd_node *keys, *key;

	keys = lydx_get_descendant(*d->diff, "keystore", "asymmetric-keys", "asymmetric-key", NULL);
	LYX_LIST_FOR_EACH(keys, key, "asymmetric-key") {
		const char *name = lydx_get_cattr(key, "name");

		if (lydx_get_xpathf(d->config, "/infix-services:ssh/hostkey[.='%s']", name))
			dep_add(d, name, "/infix-services:ssh/hostkey[.='%s']", name);

		if (lydx_get_xpathf(d->config, "/infix-services:web/certificate[.='%s']", name))
			dep_add(d, name, "/infix-services:web/certificate");

		dep_add_ifaces(d, NULL, "infix-interfaces:wireguard",
			       "infix-interfaces:wireguard/private-key='%s'", name);
	}
}

static void dep_hostname(struct dep *d)
{
	struct lyd_node *node;

	if (!lydx_get_xpathf(*d->diff, "/ietf-system:system/hostname"))
		return;

	node = lydx_get_xpathf(d->config, "/infix-dhcp-server:dhcp-server/enabled");
	if (node && lydx_is_enabled(node, "enabled"))
		dep_add(d, "true", "/infix-dhcp-server:dhcp-server/enabled");

	node = lydx_get_xpathf(d->config, "/infix-services:mdns");
	if (node && lydx_is_enabled(node, "enabled"))
		dep_add(d, "true", "/infix-services:mdns/enabled");

	dep_add_ifaces(d, NULL, "ietf-ip:ipv4/infix-dhcp-client:dhcp", "ietf-ip:ipv4/infix-dhcp-client:dhcp");
}

static void dep_wifi_interfaces(struct dep *d)
{
	struct lyd_node *difs, *dif;

	difs = lydx_get_descendant(*d->diff, "interfaces", "interface", NULL);
	LYX_LIST_FOR_EACH(difs, dif, "interface") {
		const char *ifname = lydx_get_cattr(dif, "name");
		struct lyd_node *radio, *cif;
		enum iftype type;
		enum lydx_op op;

		cif = lydx_get_xpathf(d->config, IFACE "[name='%s']", ifname);

		type = iftype_from_iface(dif);
		if (type == IFT_UNKNOWN)
			type = iftype_from_iface(cif);

		if (type != IFT_WIFI)
			continue;

		/* A derived address is the interface's position on its
		 * radio, see wifi_default_addr(), so adding or removing one
		 * moves the others.  Flag those without a fixed address the
		 * way a custom-phys-address change is, netdag then recreates
		 * them.  A deleted interface is only in the diff, so take the
		 * radio from there. */
		op = lydx_get_op(dif);
		radio = lydx_get_descendant(lyd_child(dif), "wifi", "radio", NULL);
		if ((op == LYDX_OP_CREATE || op == LYDX_OP_DELETE) && radio) {
			struct ly_set *ifaces;
			uint32_t i;

			ifaces = lydx_find_xpathf(d->config, IFACE "[infix-interfaces:wifi/radio='%s']",
						  lyd_get_value(radio));
			for (i = 0; ifaces && i < ifaces->count; i++) {
				const char *name = lydx_get_cattr(ifaces->dnodes[i], "name");
				char mac[18];

				if (!strcmp(name, ifname) ||
				    !interface_get_phys_addr(ifaces->dnodes[i], mac))
					continue;

				dep_add(d, NULL, IFACE "[name='%s']/infix-interfaces:custom-phys-address", name);
			}
			ly_set_free(ifaces, NULL);
		}

		radio = lydx_get_xpathf(d->config, IFACE "[name='%s']/infix-interfaces:wifi/radio", ifname);
		if (!radio) {
			/* A WDS link has no radio of its own, follow its access point */
			struct lyd_node *ap = lydx_get_xpathf(d->config,
				IFACE "[name='%s']/infix-interfaces:wifi/wds-link/access-point", ifname);

			if (ap)
				radio = lydx_get_xpathf(d->config, IFACE "[name='%s']/infix-interfaces:wifi/radio",
							lyd_get_value(ap));
		}
		if (!radio)
			continue;

		dep_add(d, lyd_get_value(radio), RADIO, lyd_get_value(radio));

		/* The band steering directive of a 2.4 GHz radio names the
		 * same-SSID access point on another radio, so a change to
		 * any access point regenerates every radio that has one. */
		if (lydx_get_descendant(lyd_child(dif), "wifi", "access-point", NULL) ||
		    (cif && lydx_get_descendant(lyd_child(cif), "wifi", "access-point", NULL))) {
			struct ly_set *radios;
			uint32_t i;

			radios = lydx_find_xpathf(d->config, IFACE "/infix-interfaces:wifi[access-point]/radio");
			for (i = 0; radios && i < radios->count; i++) {
				const char *name = lyd_get_value(radios->dnodes[i]);

				dep_add(d, name, RADIO, name);
			}
			ly_set_free(radios, NULL);
		}
	}
}

static void dep_radio_components(struct dep *d)
{
	struct lyd_node *components, *component;

	components = lydx_get_descendant(*d->diff, "hardware", "component", NULL);
	LYX_LIST_FOR_EACH(components, component, "component") {
		const char *name = lydx_get_cattr(component, "name");
		const char *class;
		struct ly_set *radios;
		uint32_t i;

		class = lydx_get_cattr(component, "class");
		if (!class) {
			struct lyd_node *node = lydx_get_xpathf(d->config,
				"/ietf-hardware:hardware/component[name='%s']/class", name);

			if (node)
				class = lyd_get_value(node);
		}

		if (!class || strcmp(class, "infix-hardware:wifi"))
			continue;

		radios = lydx_find_xpathf(d->config, IFACE "/infix-interfaces:wifi/radio[.='%s']", name);
		for (i = 0; radios && i < radios->count; i++) {
			struct lyd_node *wifi = lyd_parent(radios->dnodes[i]);
			const char *ifname = lydx_get_cattr(lyd_parent(wifi), "name");

			dep_add(d, NULL, IFACE "[name='%s']/infix-interfaces:wifi", ifname);
			dep_add(d, name, IFACE "[name='%s']/infix-interfaces:wifi/radio", ifname);

			if (lydx_get_child(wifi, "station"))
				dep_add(d, NULL, IFACE "[name='%s']/infix-interfaces:wifi/station", ifname);
			else if (lydx_get_child(wifi, "access-point"))
				dep_add(d, NULL, IFACE "[name='%s']/infix-interfaces:wifi/access-point", ifname);
		}
		ly_set_free(radios, NULL);
	}
}

/*
 * A bridge generates the VLAN memberships of its ports from its own
 * diff, but netdag may recreate a port that is in the diff, e.g. for
 * a new address or tunnel setting.  Pull the port's entries in the
 * bridge's VLAN list into the diff so the bridge adds them again; for
 * a port that is only modified the repeated add changes nothing.
 */
static void dep_bridge_port_vlans(struct dep *d)
{
	static const char *modes[] = { "tagged", "untagged", NULL };
	struct lyd_node *difs, *dif;

	difs = lydx_get_descendant(*d->diff, "interfaces", "interface", NULL);
	LYX_LIST_FOR_EACH(difs, dif, "interface") {
		const char *ifname = lydx_get_cattr(dif, "name");
		struct lyd_node *bridge;
		const char **mode;

		if (lydx_get_op(dif) == LYDX_OP_DELETE)
			continue;

		bridge = lydx_get_xpathf(d->config, IFACE "[name='%s']/infix-interfaces:bridge-port/bridge", ifname);
		if (!bridge)
			continue;

		for (mode = modes; *mode; mode++) {
			struct ly_set *ports;
			uint32_t i;

			ports = lydx_find_xpathf(d->config,
				IFACE "[name='%s']/infix-interfaces:bridge/vlans/vlan/%s[.='%s']",
				lyd_get_value(bridge), *mode, ifname);
			for (i = 0; ports && i < ports->count; i++) {
				const char *vid = lydx_get_cattr(lyd_parent(ports->dnodes[i]), "vid");

				dep_add(d, NULL, IFACE "[name='%s']/infix-interfaces:bridge/vlans/vlan[vid='%s']/%s[.='%s']",
					lyd_get_value(bridge), vid, *mode, ifname);
			}
			ly_set_free(ports, NULL);
		}
	}
}

static void dep_schedule_consumers(struct dep *d)
{
	const struct cron_consumer *consumer;
	size_t i;

	for (i = 0; (consumer = schedule_consumer(i)); i++) {
		struct lyd_node *node;
		const char *name;

		node = lydx_get_xpathf(*d->diff, "%s", consumer->path);
		if (!node)
			continue;

		/* A diff that only toggles 'enabled' keeps the ref in config. */
		name = lydx_get_cattr(node, "schedule");
		if (!name) {
			node = lydx_get_xpathf(d->config, "%s", consumer->path);
			name = node ? lydx_get_cattr(node, "schedule") : NULL;
		}
		if (!name)
			continue;

		dep_add(d, NULL, "/ietf-system:system/infix-schedule:schedules/schedule[name='%s']", name);
	}
}

static void (*const dep_rules[])(struct dep *) = {
	dep_symmetric_keys,
	dep_asymmetric_keys,
	dep_hostname,
	dep_wifi_interfaces,
	dep_radio_components,
	dep_bridge_port_vlans,
	dep_schedule_consumers,
};

/* Pull what depends on the changes in diff into it, until nothing more is added */
int dep_resolve(struct lyd_node **diff, struct lyd_node *config)
{
	struct dep d = { .diff = diff, .config = config };
	int pass;

	for (pass = 0; pass < DEP_MAX_PASSES; pass++) {
		size_t i;

		d.added = 0;
		for (i = 0; i < NELEMS(dep_rules); i++)
			dep_rules[i](&d);

		if (d.err)
			return -1;
		if (!d.added)
			return 0;
	}

	ERROR("Max dependency depth reached");
	return -1;
}
