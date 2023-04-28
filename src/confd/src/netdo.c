/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <stdarg.h>

#include <libite/lite.h>
#include <libite/queue.h>
#include <libyang/libyang.h>

#include "core.h"
#include "lyx.h"
#include "netdo.h"

const char *netdo_phase_str[] = {
#define netdo_phase_str_gen(_ph, _name, ...) \
	[NETDO_ ## _ph] = _name,

	NETDO_PHASES(netdo_phase_str_gen)
#undef netdo_phase_str_gen
};

const char *netdo_cmd_str[] = {
#define netdo_cmd_str_gen(_cmd, _name, ...) \
	[NETDO_ ## _cmd] = _name,

	NETDO_CMDS(netdo_cmd_str_gen)
#undef netdo_cmd_str_gen
};

static const char *netpath = "/run/net";

static struct netdo_iface *netdo_iface_get(struct netdo *nd, const char *ifname)
{
	struct netdo_iface *iface;

	TAILQ_FOREACH(iface, &nd->ifaces, node) {
		if (!strcmp(iface->name, ifname))
			return iface;
	}

	return NULL;
}

static void netdo_iface_del(struct netdo *nd, struct netdo_iface *iface)
{
	enum netdo_phase phase;
	enum netdo_cmd cmd;

	for (phase = 0; phase < __NETDO_PHASE_MAX; phase++) {
		for (cmd = 0; cmd < __NETDO_CMD_MAX; cmd++) {
			if (iface->cmd[phase][cmd])
				fclose(iface->cmd[phase][cmd]);
		}
	}

	TAILQ_REMOVE(&nd->ifaces, iface, node);
	free(iface);
}

static int netdo_iface_add(struct netdo *nd, const char *ifname)
{
	struct netdo_iface *iface;

	iface = calloc(1, sizeof(*iface));
	if (!iface)
		return -ENOMEM;

	strlcpy(iface->name, ifname, sizeof(iface->name));

	TAILQ_INSERT_TAIL(&nd->ifaces, iface, node);
	return 0;
}

FILE *netdo_get_file(struct netdo *nd, const char *ifname,
		     enum netdo_phase phase, enum netdo_cmd cmd)
{
	struct netdo_iface *iface;
	FILE *fp;

	if (!nd->path[phase][0])
		return NULL;

	iface = netdo_iface_get(nd, ifname);
	if (!iface)
		return NULL;

	if (iface->cmd[phase][cmd])
		return iface->cmd[phase][cmd];

	fp = fopenf("w", "%s/%s/%s%s", nd->path[phase], ifname,
		    netdo_phase_str[phase], netdo_cmd_str[cmd]);
	if (!fp)
		return NULL;

	switch (cmd) {
	case NETDO_ETHTOOL:
		if (fputs("#!/bin/sh\n", fp) < 0) {
			fclose(fp);
			return NULL;
		}
		break;
	default:
		break;
	}

	iface->cmd[phase][cmd] = fp;
	return fp;
}

static int netdo_open_iface(struct netdo *nd, struct lyd_node *ifdata)
{
	const char *ena = lydx_get_cattr(ifdata, "enabled");
	const char *name = lydx_get_cattr(ifdata, "name");
	char path[PATH_MAX];
	FILE *fp;

	DEBUG("%s", name ? : "INVALID");

	if (!name || !ena)
		return -EINVAL;

	strlcpy(path, nd->path[NETDO_INIT], sizeof(path));
	strlcat(path, "/", sizeof(path));
	strlcat(path, name, sizeof(path));

	if (makedir(path, 0755))
		return -EIO;

	strlcat(path, "/admin-state", sizeof(path));
	fp = fopen(path, "w");
	if (!fp)
		return -EIO;

	if (!strcmp(ena, "true"))
		fputs("up\n", fp);
	else
		fputs("disabled\n", fp);

	fclose(fp);

	return netdo_iface_add(nd, name);
}

int netdo_change(struct netdo *nd, struct lyd_node *cifs, struct lyd_node *difs)
{
	struct lyd_node *ifdata;
	int err = 0;

	memset(nd, 0, sizeof(*nd));

	nd->next_fp = fopenf("wx", "%s/next", netpath);
	if (!nd->next_fp) {
		ERROR("Transaction already in progress");
		return -1;
	}

	TAILQ_INIT(&nd->ifaces);

	if (readdf(&nd->gen, "%s/gen", netpath))
		nd->gen = 0;
	else
		nd->gen++;

	snprintf(nd->path[NETDO_INIT], sizeof(nd->path[NETDO_INIT]),
		 "%s/%d", netpath, nd->gen);
	if (makedir(nd->path[NETDO_INIT], 0755))
		return -1;

	if (nd->gen) {
		snprintf(nd->path[NETDO_EXIT], sizeof(nd->path[NETDO_EXIT]),
			 "%s/%d", netpath, nd->gen - 1);
		if (access(nd->path[NETDO_EXIT], X_OK))
			return -1;
	}
	LYX_LIST_FOR_EACH(cifs, ifdata, "interface") {
		err = netdo_open_iface(nd, ifdata);
		if (err)
			break;
	}

	LYX_LIST_FOR_EACH(difs, ifdata, "interface") {
		if (lydx_get_op(ifdata) != LYDX_OP_DELETE)
			continue;

		err = netdo_iface_add(nd, lydx_get_cattr(ifdata, "name"));
		if (err)
			break;
	}

	DEBUG("gen:%d", nd->gen);
	return err;
}

int netdo_done(struct netdo *nd)
{
	struct netdo_iface *iface, *tmp;

	DEBUG("");

	TAILQ_FOREACH_SAFE(iface, &nd->ifaces, node, tmp) {
		netdo_iface_del(nd, iface);
	}

	fprintf(nd->next_fp, "%d\n", nd->gen);
	fclose(nd->next_fp);
	return 0;
}

int netdo_abort(struct netdo *nd)
{
	struct netdo_iface *iface, *tmp;

	DEBUG("");

	TAILQ_FOREACH_SAFE(iface, &nd->ifaces, node, tmp) {
		netdo_iface_del(nd, iface);
	}

	fclose(nd->next_fp);
	return systemf("rm -rf %s/next %s", netpath, nd->path[NETDO_INIT]);
}

int netdo_boot(void)
{
	const char *envnetpath;

	envnetpath = getenv("NET_DIR");
	if (envnetpath)
		netpath = envnetpath;

	if (access(netpath, X_OK)) {
		if (makedir(netpath, 0755))
			return -1;
	}

	return 0;
}
