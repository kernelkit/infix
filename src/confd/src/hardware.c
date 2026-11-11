/* SPDX-License-Identifier: BSD-3-Clause */
#include <fnmatch.h>
#include <glob.h>
#include <jansson.h>
#include <unistd.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include "hardware.h"
#include "dagger.h"

#define GPSD_CONF                "/etc/finit.d/available/gpsd.conf"
#define GPSD_CONF_NEXT           GPSD_CONF"+"
#define GPSD_MAX_DEVICES         4
#define GPS_RULES                "/run/udev/rules.d/20-gps-%s.rules"
#define GPS_RULES_GLOB           "/run/udev/rules.d/20-gps-*.rules"
#define MODEM_RULES              "/run/udev/rules.d/95-modem-%s.rules"
#define MODEM_RULES_GLOB         "/run/udev/rules.d/95-modem-*.rules"
#define MODEM_MANAGER_ENV        "/etc/default/modem-manager"
#define MODEM_MAX                16

static char *component_xpath(const char *xpath)
{
	char *path, *ptr;

	if (!xpath)
		return NULL;

	path = strdup(xpath);
	if (!path)
		return NULL;

	if (!(ptr = strstr(path, "]/"))) {
		free(path);
		return NULL;
	}
	ptr[1] = 0;

	return path;
}

static int hardware_cand_infer_class(json_t *root, sr_session_ctx_t *session, const char *path)
{
	sr_val_t inferred = { .type = SR_STRING_T };
	struct json_t *usb_ports, *usb_port;
	sr_error_t err = SR_ERR_OK;
	char *name, *class;
	char *xpath;
	int index;

	xpath = component_xpath(path);
	if (!xpath)
		return SR_ERR_SYS;

	class = srx_get_str(session, "%s/class", xpath);
	if (class) {
		free(class);
		goto out_free_xpath;
	}

	name = srx_get_str(session, "%s/name", xpath);
	if (!name) {
		err = SR_ERR_INTERNAL;
		goto out_free_xpath;
	}
	usb_ports = json_object_get(root, "usb-ports");
	if (!usb_ports)
		goto out_free_name; /* No USB-ports is OK */

	json_array_foreach(usb_ports, index, usb_port) {
		struct json_t *n = json_object_get(usb_port, "name");
		if (!n || !json_is_string(n)) {
			ERROR("Did not find hardware port for %s", name);
			continue;
		}
		if (!strcmp(name, json_string_value(n))) {
			inferred.data.string_val = "infix-hardware:usb";
			err = srx_set_item(session, &inferred, 0,
					   "%s/class", xpath);
			goto out_free_name;
		}
	}

	if (!fnmatch("radio+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:wifi";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}

	if (!fnmatch("gps+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:gps";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}

	if (!fnmatch("modem+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:modem";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}

	if (!fnmatch("sim+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:sim";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}

out_free_name:
	free(name);
out_free_xpath:
	free(xpath);
	return err;
}

static int hardware_cand(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			 const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	sr_change_iter_t *iter;
	sr_change_oper_t op;
	sr_val_t *old, *new;
	sr_error_t err = SR_ERR_OK;
	struct confd *confd = (struct confd *)priv;

	switch (event) {
	case SR_EV_UPDATE:
	case SR_EV_CHANGE:
		break;
	default:
		return SR_ERR_OK;
	}
	err = sr_dup_changes_iter(session, "/ietf-hardware:hardware/component//*", &iter);
	if (err)
		return err;
	while (sr_get_change_next(session, iter, &op, &old, &new) == SR_ERR_OK) {
		switch (op) {
		case SR_OP_CREATED:
		case SR_OP_MODIFIED:
			break;
		default:
			continue;
		}
		err = hardware_cand_infer_class(confd->root, session, new->xpath);
		if (err)
			break;
	}
	sr_free_change_iter(iter);
	return err;
}

/*
 * USB port token from a modem component's uri leaf-list, e.g.
 * "usb:///sys/bus/usb/devices/3-1.2.1" -> "3-1.2.1".  Other URI
 * schemes (CLEI URNs, ...) are ignored.  Returns NULL when the
 * component carries no usb: URI.
 */
static const char *modem_usb_port(struct lyd_node *cif, char *buf, size_t len)
{
	struct lyd_node *node;

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "uri") {
		const char *uri = lyd_get_value(node);
		char tmp[256];
		char *port;
		size_t n;

		if (!uri || strncmp(uri, "usb:", 4))
			continue;

		strlcpy(tmp, uri, sizeof(tmp));
		n = strlen(tmp);
		while (n && tmp[n - 1] == '/')
			tmp[--n] = 0;

		port = strrchr(tmp, '/');
		port = port ? port + 1 : tmp + strlen("usb:");
		if (!port[0] || strspn(port, "0123456789.-") != strlen(port)) {
			ERROR("Malformed usb URI '%s', expected .../<port>", uri);
			continue;
		}

		strlcpy(buf, port, len);
		return buf;
	}

	return NULL;
}

/*
 * True when the diff touches a wwan interface: the generated udev
 * rule embeds the bound interface name, so it must be regenerated on
 * interface changes too, not only on hardware-component changes.
 */
static int modem_wwan_diff(struct lyd_node *diff)
{
	struct lyd_node *ifaces, *iface;

	ifaces = lydx_get_descendant(diff, "interfaces", "interface", NULL);
	LYX_LIST_FOR_EACH(ifaces, iface, "interface") {
		if (lydx_get_child(iface, "wwan"))
			return 1;
	}

	return 0;
}

/*
 * First interface bound to this modem component via the wwan/modem
 * leafref, i.e. the name the kernel netdev should carry.
 */
static const char *modem_iface_name(struct lyd_node *config, const char *modem)
{
	struct lyd_node *ifaces, *iface;

	ifaces = lydx_get_descendant(config, "interfaces", "interface", NULL);
	LYX_LIST_FOR_EACH(ifaces, iface, "interface") {
		struct lyd_node *wwan = lydx_get_child(iface, "wwan");
		const char *ref = wwan ? lydx_get_cattr(wwan, "modem") : NULL;

		if (ref && !strcmp(ref, modem))
			return lydx_get_cattr(iface, "name");
	}

	return NULL;
}

/*
 * First KERNEL=="<port>" token from a previously generated rules file,
 * so ports of removed components can be re-triggered to drop their
 * stale ID_MM_DEVICE_PROCESS tag from the udev database.
 */
static int modem_rule_port(const char *path, char *buf, size_t len)
{
	char line[256];
	FILE *fp;

	fp = fopen(path, "r");
	if (!fp)
		return -1;

	while (fgets(line, sizeof(line), fp)) {
		char *ptr = strstr(line, "KERNELS==\"");
		char *end;

		if (ptr)
			ptr += strlen("KERNELS==\"");
		else if ((ptr = strstr(line, "KERNEL==\"")))
			ptr += strlen("KERNEL==\"");
		else
			continue;
		end = strchr(ptr, '"');
		if (!end)
			continue;
		*end = 0;
		strlcpy(buf, ptr, len);
		fclose(fp);
		return 0;
	}

	fclose(fp);
	return -1;
}

/*
 * udevadm trigger treats a path argument as parent-match: the device
 * and all its children (net, tty, usbmisc) are re-triggered in one
 * call.  It bails without triggering anything if the path cannot be
 * opened, hence the existence guard for absent hardware.
 */
static void modem_trigger_port(const char *port)
{
	if (!fexistf("/sys/bus/usb/devices/%s", port))
		return;

	systemf("udevadm trigger -c add /sys/bus/usb/devices/%s 2>/dev/null", port);
}

static int modem_port_find(char (*ports)[64], int num, const char *port)
{
	int p;

	for (p = 0; p < num; p++) {
		if (!strcmp(ports[p], port))
			return p;
	}

	return -1;
}

/*
 * A GPS receiver anchored with a usb: URI gets a rules file naming its
 * tty /dev/<name>, and the gpsd rules leave it alone, so the name no
 * longer depends on enumeration order.
 */
static void gps_gen_rules(struct lyd_node *config)
{
	char trig[GPSD_MAX_DEVICES * 2][64];
	char used[GPSD_MAX_DEVICES][64];
	int num_trig = 0, num_used = 0;
	struct lyd_node *cifs, *comp;
	glob_t gl = { 0 };
	size_t i;
	int p;

	if (glob(GPS_RULES_GLOB, 0, NULL, &gl) == 0) {
		for (i = 0; i < gl.gl_pathc; i++) {
			if (num_trig < GPSD_MAX_DEVICES &&
			    !modem_rule_port(gl.gl_pathv[i], trig[num_trig], sizeof(trig[0])))
				num_trig++;
			unlink(gl.gl_pathv[i]);
		}
	}
	globfree(&gl);

	cifs = lydx_get_descendant(config, "hardware", "component", NULL);
	LYX_LIST_FOR_EACH(cifs, comp, "component") {
		const char *cls = lydx_get_cattr(comp, "class");
		const char *name;
		char port[64];
		FILE *fp;

		if (!cls || strcmp(cls, "infix-hardware:gps"))
			continue;
		if (!modem_usb_port(comp, port, sizeof(port)))
			continue;

		name = lydx_get_cattr(comp, "name");
		if (modem_port_find(used, num_used, port) >= 0) {
			ERROR("GPS %s: USB port %s already claimed, ignoring", name, port);
			continue;
		}
		if (num_used < GPSD_MAX_DEVICES)
			strlcpy(used[num_used++], port, sizeof(used[0]));
		if (num_trig < GPSD_MAX_DEVICES * 2 &&
		    modem_port_find(trig, num_trig, port) < 0)
			strlcpy(trig[num_trig++], port, sizeof(trig[0]));

		if (!fexistf("/sys/bus/usb/devices/%s", port))
			NOTE("GPS %s: no device at USB port %s (yet)", name, port);

		fp = fopenf("w", GPS_RULES, name);
		if (!fp) {
			ERRNO("Failed creating udev rules for %s", name);
			continue;
		}

		fprintf(fp, "# Generated by confd, do not edit.\n");
		fprintf(fp, "SUBSYSTEM==\"tty\", KERNELS==\"%s\", ENV{INFIX_GPS}=\"%s\", "
			"SYMLINK+=\"%s\", OPTIONS+=\"link_priority=100\"\n", port, name, name);
		fclose(fp);
	}

	for (p = 0; p < num_trig; p++)
		modem_trigger_port(trig[p]);
}

/*
 * Regenerate udev rules and the ModemManager environment from the full
 * configuration, then hand tagged devices over to ModemManager.
 *
 * Components anchored with a usb: URI get a rules file that (a) tags
 * the USB device so ModemManager may probe it and (b) names the data
 * netdev, at add time, after the interface configured for the modem.
 * A netdev that appears after config apply (dummy placeholder in the
 * way, or brought up already) keeps its kernel name until reboot,
 * same recovery model as WiFi radios.  When every unlocked modem is
 * anchored, ModemManager is switched to ALLOWLIST-ONLY filter policy
 * so it never touches unconfigured devices.  Unanchored (legacy)
 * configurations keep ModemManager's default probe behavior.
 */
static int modem_gen_rules(struct lyd_node *config)
{
	char trig[MODEM_MAX * 2][64];
	char used[MODEM_MAX][64];
	int num_trig = 0, num_used = 0;
	int unlocked = 0, unanchored = 0;
	struct lyd_node *cifs, *comp;
	glob_t gl = { 0 };
	size_t i;
	int p;
	FILE *fp;

	/* Stale rules from removed/renamed components: collect their
	 * ports for re-trigger, then drop the files. */
	if (glob(MODEM_RULES_GLOB, 0, NULL, &gl) == 0) {
		for (i = 0; i < gl.gl_pathc; i++) {
			if (num_trig < MODEM_MAX &&
			    !modem_rule_port(gl.gl_pathv[i], trig[num_trig],
					     sizeof(trig[0])))
				num_trig++;
			unlink(gl.gl_pathv[i]);
		}
	}
	globfree(&gl);

	cifs = lydx_get_descendant(config, "hardware", "component", NULL);
	LYX_LIST_FOR_EACH(cifs, comp, "component") {
		const char *cls = lydx_get_cattr(comp, "class");
		const char *name, *admin_state, *ifname;
		struct lyd_node *state;
		char port[64];

		if (!cls || strcmp(cls, "infix-hardware:modem"))
			continue;

		state = lydx_get_child(comp, "state");
		admin_state = lydx_get_cattr(state, "admin-state");
		if (!admin_state || strcmp(admin_state, "unlocked"))
			continue;

		name = lydx_get_cattr(comp, "name");

		if (!modem_usb_port(comp, port, sizeof(port))) {
			unlocked++;
			unanchored++;
			continue;
		}

		/* modemd derives its state paths from the modemN name */
		if (fnmatch("modem+([0-9])", name, FNM_EXTMATCH)) {
			ERROR("Modem %s: anchored components must be named modemN, ignoring", name);
			continue;
		}

		if (modem_port_find(used, num_used, port) >= 0) {
			ERROR("Modem %s: USB port %s already claimed, ignoring", name, port);
			continue;
		}
		if (num_used < MODEM_MAX)
			strlcpy(used[num_used++], port, sizeof(used[0]));
		if (num_trig < MODEM_MAX * 2 &&
		    modem_port_find(trig, num_trig, port) < 0)
			strlcpy(trig[num_trig++], port, sizeof(trig[0]));

		unlocked++;

		if (!fexistf("/sys/bus/usb/devices/%s", port))
			NOTE("Modem %s: no device at USB port %s (yet)", name, port);

		fp = fopenf("w", MODEM_RULES, name);
		if (!fp) {
			ERRNO("Failed creating udev rules for %s", name);
			continue;
		}

		fprintf(fp, "# Generated by confd, do not edit.\n");
		fprintf(fp, "SUBSYSTEM==\"usb\", KERNEL==\"%s\", ENV{ID_MM_DEVICE_PROCESS}=\"1\"\n",
			port);

		/* Name via a variable: eudev's rule generator renames any new
		 * interface holding a literal NAME= value, our placeholder too */
		ifname = modem_iface_name(config, name);
		if (ifname)
			fprintf(fp, "ACTION==\"add\", SUBSYSTEM==\"net\", KERNELS==\"%s\", "
				"ENV{INFIX_MODEM_IFNAME}=\"%s\", NAME=\"$env{INFIX_MODEM_IFNAME}\"\n",
				port, ifname);
		fclose(fp);
	}

	if (writesf(unlocked && !unanchored
		    ? "# Generated by confd, do not edit.\n"
		      "MODEM_MANAGER_OPTS=\"--filter-policy=allowlist-only\""
		    : "# Generated by confd, do not edit.\n"
		      "MODEM_MANAGER_OPTS=\"\"",
		    "w", MODEM_MANAGER_ENV))
		ERRNO("Failed writing " MODEM_MANAGER_ENV);

	systemf("udevadm control --reload");

	/* Re-run rules for current and removed ports alike: tags must be
	 * in the udev db before ModemManager starts, and stale tags must
	 * be dropped before it does. */
	for (p = 0; p < num_trig; p++)
		modem_trigger_port(trig[p]);

	return unlocked;
}

int hardware_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
		    sr_event_t event, struct confd *confd)
{
	struct lyd_node  *difs = NULL, *dif = NULL;
	int rc = SR_ERR_OK;
	int gps_changed = 0;
	int modem_changed = 0;
	int modem_rules_only = 0;

	if (!lydx_find_xpathf(diff, XPATH_BASE_)) {
		/* The generated udev rule embeds the bound interface
		 * name; regenerate rules (but leave services alone) when
		 * a wwan interface changes. */
		if (!modem_wwan_diff(diff))
			return SR_ERR_OK;
		modem_rules_only = 1;
	}

	/*
	 * The country code is one setting for every radio.  Apply the
	 * regulatory domain here, not from hostapd, so a radio without an
	 * interface is in the right domain too.
	 */
	if (event == SR_EV_DONE && lydx_get_xpathf(diff, XPATH_BASE_ "/infix-hardware:wifi/country-code"))
		systemf("iw reg set %s", wifi_country_code(config) ?: "00");

	difs = lydx_get_descendant(diff, "hardware", "component", NULL);
	LYX_LIST_FOR_EACH(difs, dif, "component") {
		struct lyd_node *state, *cif;
		const char *admin_state;
		const char *class, *name;

		name = lydx_get_cattr(dif, "name");

		/* Get the current config node for this component */
		cif = lydx_get_xpathf(config, "/hardware/component[name='%s']", name);
		if (!cif) {
			/* Component deleted: lock its USB port, modem services
			 * and udev rules are regenerated from the remaining
			 * config below. */
			class = lydx_get_cattr(dif, "class");
			if (event == SR_EV_DONE && class && !strcmp(class, "infix-hardware:usb") &&
			    usb_authorize(confd->root, name, 0)) {
				rc = SR_ERR_INTERNAL;
				goto err;
			}
			if (class && !strcmp(class, "infix-hardware:modem"))
				modem_changed = 1;
			if (class && !strcmp(class, "infix-hardware:gps"))
				gps_changed = 1;
			continue;
		}

		class = lydx_get_cattr(cif, "class");
		/* Handle USB components */
		if (!strcmp(class, "infix-hardware:usb")) {
			if (event != SR_EV_DONE)
				continue;

			state = lydx_get_child(cif, "state");
			admin_state = lydx_get_cattr(state, "admin-state");
			if (usb_authorize(confd->root, name,
					  admin_state && !strcmp(admin_state, "unlocked"))) {
				rc = SR_ERR_INTERNAL;
				goto err;
			}
		} else if (!strcmp(class, "infix-hardware:modem")) {
			/* Handled from full config after the loop; the
			 * services and udev rules span all modems. */
			modem_changed = 1;
		} else if (!strcmp(class, "infix-hardware:wifi")) {
			struct lyd_node **wifi_iface_list = NULL;
			struct lyd_node *interfaces_config;
			struct lyd_node *cwifi_radio;
			int wifi_iface_count = 0;

			switch (event) {
			case SR_EV_ABORT:
				continue;
			case SR_EV_CHANGE:
				break;
			default:
				continue;

			}

			cwifi_radio = lydx_get_child(cif, "wifi-radio");

			interfaces_config = lydx_get_descendant(config, "interfaces", "interface", NULL);

			wifi_find_interfaces_on_radio(interfaces_config, name,
						       &wifi_iface_list, &wifi_iface_count);

			if (wifi_iface_list) {
				if (wifi_iface_count) {
					/* Generate AP config (hostapd) for all APs on this radio */
					rc = wifi_gen_aps_on_radio(name, interfaces_config, cwifi_radio, config, confd);
					if (rc != SR_ERR_OK)
						ERROR("Failed to generate AP config for radio %s", name);
				}
				free(wifi_iface_list);
				wifi_iface_list = NULL;
				wifi_iface_count = 0;
			}
		} else if (!strcmp(class, "infix-hardware:gps")) {
			gps_changed = 1;
		}
	}

	if (gps_changed) {
		const char *gps_names[GPSD_MAX_DEVICES] = { NULL };
		struct lyd_node *cifs, *comp;
		int num_gps = 0;

		switch (event) {
		case SR_EV_UPDATE:
		case SR_EV_ENABLED:
		case SR_EV_RPC:
			break;

		case SR_EV_ABORT:
			unlink(GPSD_CONF_NEXT);
			break;

		case SR_EV_CHANGE:
			cifs = lydx_get_descendant(config, "hardware", "component", NULL);
			LYX_LIST_FOR_EACH(cifs, comp, "component") {
				const char *cls = lydx_get_cattr(comp, "class");

				if (strcmp(cls, "infix-hardware:gps"))
					continue;
				if (num_gps < GPSD_MAX_DEVICES)
					gps_names[num_gps] = lydx_get_cattr(comp, "name");
				else
					return SR_ERR_INTERNAL;
				num_gps++;
			}
			if (num_gps > 0) {
				FILE *fp;

				fp = fopenp(GPSD_CONF_NEXT, 0644, NULL);
				if (!fp) {
					ERRNO("Could not open " GPSD_CONF_NEXT);
					return SR_ERR_INTERNAL;
				}
				int i;

				fprintf(fp, "# Generated by confd, do not edit.\n");
				fprintf(fp, "service");
				fprintf(fp, " <!");
				for (i = 0; i < num_gps && i < GPSD_MAX_DEVICES; i++)
					fprintf(fp, "%sdev/%s", i ? "," : "", gps_names[i]);
				fprintf(fp, ">");
				fprintf(fp, " pid:!/run/gpsd.pid");
				fprintf(fp, " env:-/etc/default/gpsd \\\n");
				fprintf(fp, "\t[2345] gpsd -n");
				for (i = 0; i < num_gps && i < GPSD_MAX_DEVICES; i++)
					fprintf(fp, " /dev/%s", gps_names[i]);
				fprintf(fp, " -P /run/gpsd.pid -- GPS Daemon\n");
				fclose(fp);
			}
			break;

		case SR_EV_DONE:
			gps_gen_rules(config);
			if (fexist(GPSD_CONF_NEXT)) {
				if (rename(GPSD_CONF_NEXT, GPSD_CONF))
					ERRNO("Failed switching to new %s", GPSD_CONF);
				else
					finit_enable("gpsd");
			} else {
				unlink(GPSD_CONF);
				finit_disable("gpsd");
			}
			break;
		}
	}

	/*
	 * Modem components: regenerate udev anchor rules and the
	 * ModemManager environment from the full config, then drive the
	 * services.  modemd owns the bearer lifecycle and cleanly
	 * disconnects before ModemManager goes down, hence both services
	 * are driven together, from the aggregate state of all modems.
	 * On wwan-interface-only changes the rules are refreshed but the
	 * services are left alone; modemd is signaled by the interface
	 * dagger script.
	 */
	if ((modem_changed || modem_rules_only) && event == SR_EV_DONE) {
		int unlocked = modem_gen_rules(config);

		if (modem_changed) {
			if (unlocked > 0) {
				NOTE("Modem(s) enabled, starting modemd");
				finit_enable("modem-manager");
				finit_enable("modemd");
				systemf("initctl -bfqn restart modem-manager");
				systemf("initctl -bfqn restart modemd");
			} else {
				NOTE("No enabled modems, disabling modemd");
				systemf("initctl -bfqn stop modemd");
				finit_disable("modemd");
				systemf("initctl -bfqn stop modem-manager");
				finit_disable("modem-manager");
			}
		}
	}
err:

	return rc;
}

int hardware_candidate_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_CHANGE(confd->cand, "ietf-hardware", XPATH_BASE_,
			SR_SUBSCR_UPDATE, hardware_cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("Init hardware failed: %s", sr_strerror(rc));
	return rc;
}
