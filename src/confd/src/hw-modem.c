/* SPDX-License-Identifier: BSD-3-Clause */
#include <fnmatch.h>
#include <glob.h>

#include <srx/common.h>
#include <srx/lyx.h>

#include "core.h"
#include "hardware.h"

#define MODEM_RULES              "/run/udev/rules.d/95-modem-%s.rules"
#define MODEM_RULES_GLOB         "/run/udev/rules.d/95-modem-*.rules"
#define MODEM_MANAGER_ENV        "/etc/default/modem-manager"
#define MODEM_MAX                16

/*
 * USB port token from a modem component's uri leaf-list, e.g.
 * "usb:///sys/bus/usb/devices/3-1.2.1" -> "3-1.2.1".  Other URI
 * schemes (CLEI URNs, ...) are ignored.  Returns NULL when the
 * component carries no usb: URI.
 */
const char *modem_usb_port(struct lyd_node *cif, char *buf, size_t len)
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
int modem_wwan_diff(struct lyd_node *diff)
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
int modem_rule_port(const char *path, char *buf, size_t len)
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
void modem_trigger_port(const char *port)
{
	if (!fexistf("/sys/bus/usb/devices/%s", port))
		return;

	systemf("udevadm trigger -c add /sys/bus/usb/devices/%s 2>/dev/null", port);
}

int modem_port_find(char (*ports)[64], int num, const char *port)
{
	int p;

	for (p = 0; p < num; p++) {
		if (!strcmp(ports[p], port))
			return p;
	}

	return -1;
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
int modem_gen_rules(struct lyd_node *config)
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
