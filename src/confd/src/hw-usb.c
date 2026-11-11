/* SPDX-License-Identifier: BSD-3-Clause */
#include <ctype.h>
#include <ftw.h>
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
