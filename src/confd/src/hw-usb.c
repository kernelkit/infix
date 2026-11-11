/* SPDX-License-Identifier: BSD-3-Clause */
#include <ctype.h>
#include <ftw.h>
#include <glob.h>
#include <jansson.h>
#include <libgen.h>
#include <limits.h>

#include <srx/lyx.h>

#include "core.h"
#include "hardware.h"

static int dir_cb(const char *fpath, const struct stat *sb, int typeflag, struct FTW *ftwbuf)
{
	const char *filename;

	if (typeflag == FTW_DP)
		return 0;

	filename = basename((char *)fpath);
	if (!strcmp(filename, "authorized_default") || !strcmp(filename, "authorized")) {
		if (writedf(1, "w", "%s", fpath)) {
			ERROR("Failed to authorize %s", fpath);
			return FTW_STOP;
		}
	}

	return 0;
}

/* Hubs stay authorized so the devices behind them stay listed */
static int is_hub(const char *dir)
{
	char class[8];

	return readsnf(class, sizeof(class), "%s/bDeviceClass", dir) &&
		!strcmp(class, "09");
}

/* Devices on ports the device tree marks as hard-wired, e.g. a modem slot */
static int is_hardwired(const char *dir)
{
	char type[16];

	return readsnf(type, sizeof(type), "%s/port/connect_type", dir) &&
		!strcmp(type, "hardwired");
}

/* Deauthorize devices (1-1.2), not interfaces (1-1.2:1.0), hubs, hard-wired
 * devices or the root hub (usb1) */
static int deauth_cb(const char *fpath, const struct stat *sb, int typeflag, struct FTW *ftwbuf)
{
	char dir[PATH_MAX];
	const char *dev;

	if (typeflag != FTW_F || strcmp(basename((char *)fpath), "authorized"))
		return 0;

	strlcpy(dir, fpath, sizeof(dir));
	dirname(dir);
	dev = basename(dir);
	if (!isdigit(dev[0]) || strchr(dev, ':') || is_hub(dir) || is_hardwired(dir))
		return 0;

	if (writedf(0, "w", "%s", fpath))
		ERROR("Failed to deauthorize %s", fpath);

	return 0;
}

static int usb_authorize_bus(const char *path, int enabled)
{
	char apath[PATH_MAX];

	snprintf(apath, sizeof(apath), "%s/authorized_default", path);

	if (!enabled) {
		/* 2: only devices on hard-wired ports, as at boot */
		if (fexist(apath) && writedf(2, "w", "%s", apath)) {
			ERROR("Failed to unauthorize %s", apath);
			return 1;
		}
		if (realpath(path, apath))
			nftw(apath, deauth_cb, 20, FTW_DEPTH | FTW_PHYS);
		return 0;
	}

	/*
	 * Set authorized_default=1 on the bus BEFORE nftw
	 * authorizes individual devices.  With FTW_DEPTH,
	 * nftw visits children before the root-hub entry, so
	 * without this a hub would be authorized first while
	 * authorized_default is still 2.  Setting it here
	 * ensures devices appearing behind an intermediate
	 * hub are auto-authorized by the kernel as they probe.
	 */
	if (fexist(apath))
		writedf(1, "w", "%s", apath);
	if (realpath(path, apath))
		nftw(apath, dir_cb, 20, FTW_DEPTH | FTW_PHYS);

	return 0;
}

static bool usb_authorize(struct json_t *root, const char *name, int enabled)
{
	json_t *port, *ports, *paths, *path;
	size_t index, i;

	ports = json_object_get(root, "usb-ports");
	if (!ports)
		return 0;

	json_array_foreach(ports, index, port) {
		struct json_t *obj;

		obj = json_object_get(port, "name");
		if (!obj || !json_is_string(obj)) {
			ERROR("Did not find USB hardware port (name) for %s", name);
			continue;
		}

		if (strcmp(name, json_string_value(obj)))
			continue;

		paths = json_object_get(port, "paths");
		if (!paths || !json_is_array(paths)) {
			ERROR("Did not find USB hardware port (paths) for %s", name);
			continue;
		}

		json_array_foreach(paths, i, path) {
			if (!json_is_string(path))
				continue;
			if (usb_authorize_bus(json_string_value(path), enabled))
				return 1;
		}
	}

	return 0;
}

/* A deleted port, one without a config node, is locked */
int usb_change(struct lyd_node *cif, const char *name, sr_event_t event, struct confd *confd)
{
	const char *admin_state;

	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	admin_state = lydx_get_cattr(lydx_get_child(cif, "state"), "admin-state");
	if (usb_authorize(confd->root, name, admin_state && !strcmp(admin_state, "unlocked")))
		return SR_ERR_INTERNAL;

	return SR_ERR_OK;
}

/*
 * USB port token from a component's uri leaf-list, e.g.
 * "usb:///sys/bus/usb/devices/3-1.2.1" -> "3-1.2.1".  Other URI
 * schemes (CLEI URNs, ...) are ignored.  Returns NULL when the
 * component carries no usb: URI.
 */
const char *usb_uri_port(struct lyd_node *cif, char *buf, size_t len)
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
 * First KERNEL=="<port>" token from a previously generated rules file,
 * so ports of removed components can be re-triggered to drop their
 * stale ID_MM_DEVICE_PROCESS tag from the udev database.
 */
static int usb_rule_port(const char *path, char *buf, size_t len)
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
static void usb_trigger_port(const char *port)
{
	if (!fexistf("/sys/bus/usb/devices/%s", port))
		return;

	systemf("udevadm trigger -c add /sys/bus/usb/devices/%s 2>/dev/null", port);
}

bool usb_ports_has(const struct usb_ports *ports, const char *port)
{
	int i;

	for (i = 0; i < ports->num; i++) {
		if (!strcmp(ports->port[i], port))
			return true;
	}

	return false;
}

void usb_ports_add(struct usb_ports *ports, const char *port)
{
	if (ports->num >= USB_PORTS_MAX || usb_ports_has(ports, port))
		return;

	strlcpy(ports->port[ports->num++], port, sizeof(ports->port[0]));
}

/* Remove generated udev rules files, their ports are added to ports */
void usb_rules_purge(const char *pattern, struct usb_ports *ports)
{
	glob_t gl = { 0 };
	char port[64];
	size_t i;

	if (glob(pattern, 0, NULL, &gl) == 0) {
		for (i = 0; i < gl.gl_pathc; i++) {
			if (!usb_rule_port(gl.gl_pathv[i], port, sizeof(port)))
				usb_ports_add(ports, port);
			unlink(gl.gl_pathv[i]);
		}
	}
	globfree(&gl);
}

void usb_ports_trigger(const struct usb_ports *ports)
{
	int i;

	for (i = 0; i < ports->num; i++)
		usb_trigger_port(ports->port[i]);
}
