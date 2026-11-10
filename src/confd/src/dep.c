/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * A change to one part of the configuration may require another part
 * to be regenerated.  The nodes of that other part are added to the
 * diff, so the module that owns them sees them as changed.
 *
 * Most dependencies are references: a leaf that names an entry
 * elsewhere.  Those are listed in dep_refs[].  What cannot be written
 * as a reference is a function in the module that owns the logic,
 * listed in dep_rules[].
 */

#include <stdarg.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "core.h"

#define DEP_MAX_PASSES 10

#define IFACE    "/ietf-interfaces:interfaces/interface"
#define WIFI     IFACE "/infix-interfaces:wifi"
#define WG       IFACE "/infix-interfaces:wireguard"
#define VLAN     IFACE "/infix-interfaces:bridge/vlans/vlan"
#define SYMKEY   "/ietf-keystore:keystore/symmetric-keys/symmetric-key"
#define ASYMKEY  "/ietf-keystore:keystore/asymmetric-keys/asymmetric-key"
#define COMP     "/ietf-hardware:hardware/component"
#define HOSTNAME "/ietf-system:system/hostname"

struct dep_ref {
	const char *changed;	/* Nodes in the diff */
	bool      (*match)(struct dep *d, struct lyd_node *node);
	const char *add;	/* Nodes in the config, %s is the name of the changed one */
};

static bool is_wifi_radio(struct dep *d, struct lyd_node *component)
{
	const char *class = lydx_get_cattr(component, "class");

	if (!class) {
		struct lyd_node *node = lydx_get_xpathf(d->config, COMP "[name='%s']/class",
							lydx_get_cattr(component, "name"));

		class = node ? lyd_get_value(node) : NULL;
	}

	return class && !strcmp(class, "infix-hardware:wifi");
}

/*
 * A changed interface that is a port of a bridge.  The bridge
 * generates the VLAN memberships of its ports from its own diff, but
 * netdag may recreate a port that is in the diff, e.g. for a new
 * address or tunnel setting, so the port's entries in the bridge's
 * VLAN list are added again; for a port that is only modified the
 * repeated add changes nothing.  A bridge without VLAN filtering has
 * no such entries and nothing is added.
 */
static bool is_changed_bridge_port(struct dep *d, struct lyd_node *iface)
{
	if (lydx_get_op(iface) == LYDX_OP_DELETE)
		return false;

	return lydx_get_xpathf(d->config, IFACE "[name='%s']/infix-interfaces:bridge-port/bridge",
			       lydx_get_cattr(iface, "name")) != NULL;
}

static const struct dep_ref dep_refs[] = {
	{ SYMKEY,   NULL,                   WIFI "/secret[.='%s']" },
	{ SYMKEY,   NULL,                   WG "[peers/preshared-key='%s']" },
	{ SYMKEY,   NULL,                   WG "[peers/peer/preshared-key='%s']" },
	{ SYMKEY,   NULL,                   IFACE "/infix-interfaces:ppp/secret[.='%s']" },

	{ ASYMKEY,  NULL,                   "/infix-services:ssh/hostkey[.='%s']" },
	{ ASYMKEY,  NULL,                   "/infix-services:web/certificate[.='%s']" },
	{ ASYMKEY,  NULL,                   WG "[private-key='%s']" },

	{ COMP,     is_wifi_radio,          WIFI "[radio='%s']" },
	{ COMP,     is_wifi_radio,          WIFI "/radio[.='%s']" },
	{ COMP,     is_wifi_radio,          WIFI "[radio='%s']/station" },
	{ COMP,     is_wifi_radio,          WIFI "[radio='%s']/access-point" },

	{ IFACE,    is_changed_bridge_port, VLAN "/tagged[.='%s']" },
	{ IFACE,    is_changed_bridge_port, VLAN "/untagged[.='%s']" },

	{ HOSTNAME, NULL,                   "/infix-services:mdns/enabled[.='true']" },
	{ HOSTNAME, NULL,                   IFACE "/ietf-ip:ipv4/infix-dhcp-client:dhcp" },
};

static void dep_add_path(struct dep *d, const char *xpath, const char *value)
{
	struct lyd_node *node = NULL, *root;
	int rc;

	if (d->err || lydx_get_xpathf(*d->diff, "%s", xpath))
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

/* Add a node to the diff, unless it is already there */
void dep_add(struct dep *d, const char *value, const char *fmt, ...)
{
	char *xpath;
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = vasprintf(&xpath, fmt, ap);
	va_end(ap);
	if (rc < 0) {
		d->err = 1;
		return;
	}

	dep_add_path(d, xpath, value);
	free(xpath);
}

/* Add a node from the config to the diff, unless it is already there */
void dep_add_node(struct dep *d, struct lyd_node *node)
{
	char *xpath = lyd_path(node, LYD_PATH_STD, NULL, 0);

	if (!xpath) {
		d->err = 1;
		return;
	}

	dep_add_path(d, xpath, lyd_get_value(node));
	free(xpath);
}

static void dep_ref(struct dep *d, const struct dep_ref *ref)
{
	struct ly_set *changed, *nodes;
	uint32_t i, j;

	changed = lydx_find_xpathf(*d->diff, "%s", ref->changed);
	for (i = 0; changed && i < changed->count; i++) {
		struct lyd_node *node = changed->dnodes[i];

		if (ref->match && !ref->match(d, node))
			continue;

		nodes = lydx_find_xpathf(d->config, ref->add, lydx_get_cattr(node, "name"));
		for (j = 0; nodes && j < nodes->count; j++)
			dep_add_node(d, nodes->dnodes[j]);
		ly_set_free(nodes, NULL);
	}
	ly_set_free(changed, NULL);
}

static void dep_references(struct dep *d)
{
	size_t i;

	for (i = 0; i < NELEMS(dep_refs); i++)
		dep_ref(d, &dep_refs[i]);
}

static void (*const dep_rules[])(struct dep *) = {
	dep_references,
	wifi_dep,
	schedule_dep,
};

/* Pull what depends on the changes in diff into it, until nothing more is added */
int dep_resolve(struct lyd_node **diff, struct lyd_node *config)
{
	struct dep d = { .diff = diff, .config = config };
	int pass;

	if (!*diff)
		return 0;

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
