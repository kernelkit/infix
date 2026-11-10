/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include <srx/lyx.h>

#include "interfaces.h"
#include "core.h"

static confd_dependency_t add_dependencies(struct lyd_node **diff, const char *xpath, const char *value)
{
	struct lyd_node *new_node = NULL;
	struct lyd_node *target = NULL;
	struct lyd_node *root = NULL;

	if (!lydx_get_xpathf(*diff, "%s", xpath)) {
		int rc;

		/* Create the path, potentially creating a new tree */
		rc = lyd_new_path(NULL, LYD_CTX(*diff), xpath, value, LYD_NEW_PATH_UPDATE, &new_node);
		if (rc != LY_SUCCESS || !new_node) {
			ERROR("lyd_new_path failed with rc=%d", rc);
			return CONFD_DEP_ERROR;
		}

		root = new_node;
		while (root->parent)
			root = lyd_parent(root);

		rc = lyd_merge_siblings(diff, root, LYD_MERGE_DESTRUCT);
		if (rc != LY_SUCCESS) {
			ERROR("lyd_merge_siblings failed with rc=%d", rc);
			lyd_free_tree(root);
			return CONFD_DEP_ERROR;
		}

		target = lydx_get_xpathf(*diff, "%s", xpath);
		if (target) {
			lyd_new_meta(LYD_CTX(target), target, NULL,
				     "yang:operation", "replace", false, NULL);
		} else {
			return CONFD_DEP_ERROR;
		}

		return CONFD_DEP_ADDED;
	}

	return CONFD_DEP_DONE;
}

static confd_dependency_t dep_symmetric_keys(struct lyd_node **diff, struct lyd_node *config)
{
	confd_dependency_t result = CONFD_DEP_DONE;
	struct lyd_node *dkeys, *dkey;

	dkeys = lydx_get_descendant(*diff, "keystore", "symmetric-keys", "symmetric-key", NULL);
	LYX_LIST_FOR_EACH(dkeys, dkey, "symmetric-key") {
		const char *key_name = lydx_get_cattr(dkey, "name");
		struct ly_set *ifaces;
		uint32_t i;

		ifaces = lydx_find_xpathf(config,
			"/ietf-interfaces:interfaces/interface[infix-interfaces:wifi/secret='%s']", key_name);
		if (ifaces && ifaces->count > 0) {
			for (i = 0; i < ifaces->count; i++) {
				const char *ifname = lydx_get_cattr(ifaces->dnodes[i], "name");
				char xpath[256];

				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/secret", ifname);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add wifi node to diff for interface %s", ifname);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}
		}
		ly_set_free(ifaces, NULL);

		ifaces = lydx_find_xpathf(config,
			"/ietf-interfaces:interfaces/interface[infix-interfaces:wireguard/peers/preshared-key='%s']", key_name);
		if (ifaces && ifaces->count > 0) {
			for (i = 0; i < ifaces->count; i++) {
				const char *ifname = lydx_get_cattr(ifaces->dnodes[i], "name");
				char xpath[256];

				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wireguard", ifname);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add wireguard to diff for interface %s", ifname);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}
		}
		ly_set_free(ifaces, NULL);

		ifaces = lydx_find_xpathf(config,
			"/ietf-interfaces:interfaces/interface[infix-interfaces:wireguard/peers/peer/preshared-key='%s']", key_name);
		if (ifaces && ifaces->count > 0) {
			for (i = 0; i < ifaces->count; i++) {
				const char *ifname = lydx_get_cattr(ifaces->dnodes[i], "name");
				char xpath[256];

				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wireguard", ifname);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add wireguard to diff for interface %s", ifname);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}
		}
		ly_set_free(ifaces, NULL);

		ifaces = lydx_find_xpathf(config,
			"/ietf-interfaces:interfaces/interface[infix-interfaces:ppp/secret='%s']", key_name);
		if (ifaces && ifaces->count > 0) {
			for (i = 0; i < ifaces->count; i++) {
				const char *ifname = lydx_get_cattr(ifaces->dnodes[i], "name");
				char xpath[256];

				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:ppp/secret", ifname);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add ppp to diff for interface %s", ifname);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}
		}
		ly_set_free(ifaces, NULL);
	}

	return result;
}

static confd_dependency_t dep_asymmetric_keys(struct lyd_node **diff, struct lyd_node *config)
{
	confd_dependency_t result = CONFD_DEP_DONE;
	struct lyd_node *dkeys, *dkey;

	dkeys = lydx_get_descendant(*diff, "keystore", "asymmetric-keys", "asymmetric-key", NULL);
	LYX_LIST_FOR_EACH(dkeys, dkey, "asymmetric-key") {
		const char *key_name = lydx_get_cattr(dkey, "name");
		struct ly_set *hostkeys, *ifaces;
		struct lyd_node *webcert;
		uint32_t i;

		hostkeys = lydx_find_xpathf(config, "/infix-services:ssh/hostkey[.='%s']", key_name);
		if (hostkeys && hostkeys->count > 0) {
			for (i = 0; i < hostkeys->count; i++) {
				char xpath[256];

				snprintf(xpath, sizeof(xpath), "/infix-services:ssh/hostkey[.='%s']", key_name);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add ssh hostkey to diff for key %s", key_name);
					ly_set_free(hostkeys, NULL);
					return result;
				}
			}
			ly_set_free(hostkeys, NULL);
		}

		webcert = lydx_get_xpathf(config, "/infix-services:web/certificate[.='%s']", key_name);
		if (webcert) {
			result = add_dependencies(diff, "/infix-services:web/certificate", key_name);
			if (result == CONFD_DEP_ERROR) {
				ERROR("Failed to add web certificate to diff for key %s", key_name);
				return result;
			}
		}

		ifaces = lydx_find_xpathf(config,
			"/ietf-interfaces:interfaces/interface[infix-interfaces:wireguard/private-key='%s']", key_name);
		if (ifaces && ifaces->count > 0) {
			for (i = 0; i < ifaces->count; i++) {
				const char *ifname = lydx_get_cattr(ifaces->dnodes[i], "name");
				char xpath[256];

				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wireguard", ifname);
				result = add_dependencies(diff, xpath, key_name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add wireguard to diff for interface %s", ifname);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}
		}
		ly_set_free(ifaces, NULL);
	}

	return result;
}

static confd_dependency_t dep_hostname(struct lyd_node **diff, struct lyd_node *config)
{
	confd_dependency_t result = CONFD_DEP_DONE;
	struct lyd_node *hostname, *mdns, *dhcp_server;
	struct ly_set *ifaces;
	uint32_t i;

	hostname = lydx_get_xpathf(*diff, "/ietf-system:system/hostname");
	if (!hostname)
		return CONFD_DEP_DONE;

	dhcp_server = lydx_get_xpathf(config, "/infix-dhcp-server:dhcp-server/enabled");
	if (dhcp_server && lydx_is_enabled(dhcp_server, "enabled")) {
		result = add_dependencies(diff, "/infix-dhcp-server:dhcp-server/enabled", "true");
		if (result == CONFD_DEP_ERROR) {
			ERROR("Failed to add dhcp-server to diff on hostname change");
			return result;
		}
	}

	mdns = lydx_get_xpathf(config, "/infix-services:mdns");
	if (mdns && lydx_is_enabled(mdns, "enabled")) {
		result = add_dependencies(diff, "/infix-services:mdns/enabled", "true");
		if (result == CONFD_DEP_ERROR) {
			ERROR("Failed to add mdns to diff on hostname change");
			return result;
		}
	}

	ifaces = lydx_find_xpathf(config,
		"/ietf-interfaces:interfaces/interface[ietf-ip:ipv4/infix-dhcp-client:dhcp]");
	if (ifaces && ifaces->count > 0) {
		for (i = 0; i < ifaces->count; i++) {
			const char *ifname = lydx_get_cattr(ifaces->dnodes[i], "name");
			char xpath[256];

			snprintf(xpath, sizeof(xpath),
				 "/ietf-interfaces:interfaces/interface[name='%s']/ietf-ip:ipv4/infix-dhcp-client:dhcp", ifname);
			result = add_dependencies(diff, xpath, ifname);
			if (result == CONFD_DEP_ERROR) {
				ERROR("Failed to add dhcp-client to diff for interface %s", ifname);
				ly_set_free(ifaces, NULL);
				return result;
			}
		}
	}
	ly_set_free(ifaces, NULL);

	return result;
}

static confd_dependency_t dep_wifi_interfaces(struct lyd_node **diff, struct lyd_node *config)
{
	confd_dependency_t result = CONFD_DEP_DONE;
	struct lyd_node *difs, *dif;

	difs = lydx_get_descendant(*diff, "interfaces", "interface", NULL);
	if (!difs)
		return CONFD_DEP_DONE;

	LYX_LIST_FOR_EACH(difs, dif, "interface") {
		struct lyd_node *radio_node, *cif;
		const char *ifname, *radio_name;
		enum iftype type;
		enum lydx_op op;
		char xpath[256];

		ifname = lydx_get_cattr(dif, "name");

		cif = lydx_get_xpathf(config, "/ietf-interfaces:interfaces/interface[name='%s']", ifname);

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
		radio_node = lydx_get_descendant(lyd_child(dif), "wifi", "radio", NULL);
		if ((op == LYDX_OP_CREATE || op == LYDX_OP_DELETE) && radio_node) {
			struct ly_set *ifaces;
			uint32_t i;

			radio_name = lyd_get_value(radio_node);
			ifaces = lydx_find_xpathf(config,
				"/ietf-interfaces:interfaces/interface[infix-interfaces:wifi/radio='%s']",
				radio_name);
			for (i = 0; ifaces && i < ifaces->count; i++) {
				const char *name = lydx_get_cattr(ifaces->dnodes[i], "name");
				char mac[18];

				if (!strcmp(name, ifname) ||
				    !interface_get_phys_addr(ifaces->dnodes[i], mac))
					continue;

				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:custom-phys-address", name);
				result = add_dependencies(diff, xpath, NULL);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add %s to diff for WiFi interface %s", name, ifname);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}
			ly_set_free(ifaces, NULL);
		}

		radio_node = lydx_get_xpathf(config,
			"/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/radio", ifname);
		if (!radio_node) {
			/* A WDS link has no radio of its own, follow its access point */
			struct lyd_node *ap = lydx_get_xpathf(config,
				"/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/wds-link/access-point",
				ifname);

			if (ap)
				radio_node = lydx_get_xpathf(config,
					"/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/radio",
					lyd_get_value(ap));
		}
		if (!radio_node)
			continue;

		radio_name = lyd_get_value(radio_node);

		/* Add the radio to the diff */
		snprintf(xpath, sizeof(xpath),
			 "/ietf-hardware:hardware/component[name='%s']/infix-hardware:wifi-radio", radio_name);
		result = add_dependencies(diff, xpath, radio_name);
		if (result == CONFD_DEP_ERROR) {
			ERROR("Failed to add radio %s to diff for WiFi interface %s", radio_name, ifname);
			return result;
		}

		DEBUG("Added radio %s to diff for WiFi interface %s", radio_name, ifname);

		/* The band steering directive of a 2.4 GHz radio names the
		 * same-SSID access point on another radio, so a change to
		 * any access point regenerates every radio that has one. */
		if (lydx_get_descendant(lyd_child(dif), "wifi", "access-point", NULL) ||
		    (cif && lydx_get_descendant(lyd_child(cif), "wifi", "access-point", NULL))) {
			struct ly_set *radios;
			uint32_t i;

			radios = lydx_find_xpathf(config,
				"/ietf-interfaces:interfaces/interface/infix-interfaces:wifi[access-point]/radio");
			for (i = 0; radios && i < radios->count; i++) {
				const char *name = lyd_get_value(radios->dnodes[i]);

				snprintf(xpath, sizeof(xpath),
					 "/ietf-hardware:hardware/component[name='%s']/infix-hardware:wifi-radio", name);
				result = add_dependencies(diff, xpath, name);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add radio %s to diff for access point %s", name, ifname);
					ly_set_free(radios, NULL);
					return result;
				}
			}
			ly_set_free(radios, NULL);
		}
	}

	return result;
}

static confd_dependency_t dep_radio_components(struct lyd_node **diff, struct lyd_node *config)
{
	confd_dependency_t result = CONFD_DEP_DONE;
	struct lyd_node *dcomponents, *dcomponent;

	dcomponents = lydx_get_descendant(*diff, "hardware", "component", NULL);
	LYX_LIST_FOR_EACH(dcomponents, dcomponent, "component") {
		const char *class, *name;
		struct ly_set *ifaces;
		uint32_t j;

		name = lydx_get_cattr(dcomponent, "name");

		class = lydx_get_cattr(dcomponent, "class");
		if (!class) {
			struct lyd_node *class_node = lydx_get_xpathf(config,
				"/ietf-hardware:hardware/component[name='%s']/class", name);
			if (class_node)
				class = lyd_get_value(class_node);
		}

		if (!class || strcmp(class, "infix-hardware:wifi"))
			continue;

		/* Find all interfaces that reference this radio */
		ifaces = lydx_find_xpathf(config,
			"/ietf-interfaces:interfaces/interface/infix-interfaces:wifi/radio[.='%s']", name);
		if (!ifaces || ifaces->count == 0) {
			ly_set_free(ifaces, NULL);
			continue;
		}

		DEBUG("Found %d interfaces using radio %s", ifaces->count, name);
		for (j = 0; j < ifaces->count; j++) {
			struct lyd_node *wifi_node = lyd_parent(ifaces->dnodes[j]);
			struct lyd_node *iface_node = lyd_parent(wifi_node);
			const char *ifname;
			char xpath[256];

			ifname = lydx_get_cattr(iface_node, "name");

			snprintf(xpath, sizeof(xpath),
				 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi", ifname);
			result = add_dependencies(diff, xpath, ifname);
			if (result == CONFD_DEP_ERROR) {
				ERROR("Failed to add interface wifi for %s (radio %s)", ifname, name);
				ly_set_free(ifaces, NULL);
				return result;
			}

			snprintf(xpath, sizeof(xpath),
				 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/radio", ifname);
			result = add_dependencies(diff, xpath, name);
			if (result == CONFD_DEP_ERROR) {
				ERROR("Failed to add radio leaf for interface %s (radio %s)", ifname, name);
				ly_set_free(ifaces, NULL);
				return result;
			}

			if (lydx_get_child(wifi_node, "station")) {
				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/station", ifname);
				result = add_dependencies(diff, xpath, ifname);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add station for interface %s (radio %s)", ifname, name);
					ly_set_free(ifaces, NULL);
					return result;
				}
			} else if (lydx_get_child(wifi_node, "access-point")) {
				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']/infix-interfaces:wifi/access-point", ifname);
				result = add_dependencies(diff, xpath, ifname);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add access-point for interface %s (radio %s)", ifname, name);
					ly_set_free(ifaces, NULL);
					return result;
				}
			}

			DEBUG("Added interface %s to diff for radio %s", ifname, name);
		}
		ly_set_free(ifaces, NULL);
	}

	return result;
}

/*
 * A bridge generates the VLAN memberships of its ports from its own
 * diff, but netdag may recreate a port that is in the diff, e.g. for
 * a new address or tunnel setting.  Pull the port's entries in the
 * bridge's VLAN list into the diff so the bridge adds them again; for
 * a port that is only modified the repeated add changes nothing.
 */
static confd_dependency_t dep_bridge_port_vlans(struct lyd_node **diff, struct lyd_node *config)
{
	static const char *modes[] = { "tagged", "untagged", NULL };
	confd_dependency_t result = CONFD_DEP_DONE;
	struct lyd_node *difs, *dif;

	difs = lydx_get_descendant(*diff, "interfaces", "interface", NULL);
	LYX_LIST_FOR_EACH(difs, dif, "interface") {
		const char *ifname = lydx_get_cattr(dif, "name");
		struct lyd_node *bridge;
		const char **mode;

		if (lydx_get_op(dif) == LYDX_OP_DELETE)
			continue;

		bridge = lydx_get_xpathf(config, "/ietf-interfaces:interfaces/interface[name='%s']"
					 "/infix-interfaces:bridge-port/bridge", ifname);
		if (!bridge)
			continue;

		for (mode = modes; *mode; mode++) {
			struct ly_set *ports;
			uint32_t i;

			ports = lydx_find_xpathf(config, "/ietf-interfaces:interfaces/interface[name='%s']"
						 "/infix-interfaces:bridge/vlans/vlan/%s[.='%s']",
						 lyd_get_value(bridge), *mode, ifname);
			for (i = 0; ports && i < ports->count; i++) {
				const char *vid = lydx_get_cattr(lyd_parent(ports->dnodes[i]), "vid");
				char xpath[256];

				snprintf(xpath, sizeof(xpath),
					 "/ietf-interfaces:interfaces/interface[name='%s']"
					 "/infix-interfaces:bridge/vlans/vlan[vid='%s']/%s[.='%s']",
					 lyd_get_value(bridge), vid, *mode, ifname);
				result = add_dependencies(diff, xpath, NULL);
				if (result == CONFD_DEP_ERROR) {
					ERROR("Failed to add VLAN %s of %s to diff for bridge port %s",
					      vid, lyd_get_value(bridge), ifname);
					ly_set_free(ports, NULL);
					return result;
				}
			}
			ly_set_free(ports, NULL);
		}
	}

	return result;
}

static confd_dependency_t dep_schedule_consumers(struct lyd_node **diff, struct lyd_node *config)
{
	confd_dependency_t result = CONFD_DEP_DONE;
	const struct cron_consumer *consumer;
	size_t i;

	for (i = 0; (consumer = schedule_consumer(i)); i++) {
		struct lyd_node *node;
		const char *name;
		char xpath[256];

		node = lydx_get_xpathf(*diff, "%s", consumer->path);
		if (!node)
			continue;

		/* A diff that only toggles 'enabled' keeps the ref in config. */
		name = lydx_get_cattr(node, "schedule");
		if (!name) {
			node = lydx_get_xpathf(config, "%s", consumer->path);
			name = node ? lydx_get_cattr(node, "schedule") : NULL;
		}
		if (!name)
			continue;

		snprintf(xpath, sizeof(xpath),
			 "/ietf-system:system/infix-schedule:schedules/schedule[name='%s']", name);
		result = add_dependencies(diff, xpath, name);
		if (result == CONFD_DEP_ERROR) {
			ERROR("Failed to add schedule %s to diff for %s", name, consumer->path);
			return result;
		}

		DEBUG("Added schedule %s to diff for %s", name, consumer->path);
	}

	return result;
}

confd_dependency_t handle_dependencies(struct lyd_node **diff, struct lyd_node *config)
{
	confd_dependency_t result;

	result = dep_symmetric_keys(diff, config);
	if (result == CONFD_DEP_ERROR)
		return result;

	result = dep_asymmetric_keys(diff, config);
	if (result == CONFD_DEP_ERROR)
		return result;

	result = dep_hostname(diff, config);
	if (result == CONFD_DEP_ERROR)
		return result;

	result = dep_wifi_interfaces(diff, config);
	if (result == CONFD_DEP_ERROR)
		return result;

	result = dep_radio_components(diff, config);
	if (result == CONFD_DEP_ERROR)
		return result;

	result = dep_bridge_port_vlans(diff, config);
	if (result == CONFD_DEP_ERROR)
		return result;

	result = dep_schedule_consumers(diff, config);
	if (result == CONFD_DEP_ERROR)
		return result;

	return result;
}
