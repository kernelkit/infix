/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <dirent.h>
#include <pwd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_module.h>
#include <srx/srx_val.h>

#include "core.h"
#define  ARPING_MSEC  1000
#define  MODULE       "infix-containers"
#define  CFG_XPATH    "/infix-containers:container"
#define  INBOX_QUEUE  "/run/containers/inbox"
#define  JOB_QUEUE    "/run/containers/queue"
#define  DONE_QUEUE   "/var/lib/containers/done"
#define  LOGGER       "logger -t container -p local1.notice"

static const struct srx_module_requirement reqs[] = {
	{ .dir = YANG_PATH_, .name = MODULE, .rev = "2023-12-14" },
	{ NULL }
};

static int add(const char *name, struct lyd_node *cif)
{
	const char *image = lydx_get_cattr(cif, "image");
	const char *restart_policy, *string;
	struct lyd_node *node;
	FILE *fp, *ap;
	char *restart = "";	/* Default restart:10 */

	fp = fopenf("w", "%s/%s.sh", INBOX_QUEUE, name);
	if (!fp) {
		ERRNO("Failed adding job %s.sh to job queue" INBOX_QUEUE, name);
		return SR_ERR_SYS;
	}

	/* Stop any running container gracefully so it releases its IP addresses. */
	fprintf(fp, "#!/bin/sh\n"
		"container stop %s\n"
		"container delete %s\n"
		"container", name, name);

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "dns")
		fprintf(fp, " --dns %s", lyd_get_value(node));

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "search")
		fprintf(fp, " --dns-search %s", lyd_get_value(node));

	if ((string = lydx_get_cattr(cif, "hostname")))
		fprintf(fp, " --hostname %s", string);

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "publish")
		fprintf(fp, " -p %s", lyd_get_value(node));

	if (lydx_is_enabled(cif, "read-only"))
		fprintf(fp, " --read-only");

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "volume")
		fprintf(fp, " -v %s-%s:%s", name, lydx_get_cattr(node, "name"), lydx_get_cattr(node, "dir"));

	ap = NULL;
	LYX_LIST_FOR_EACH(lyd_child(cif), node, "env") {
		if (!ap) {
			ap = fopenf("w", "/run/containers/args/%s.env", name);
			if (!ap) {
				ERRNO("Failed creating env list for container %s", name);
				break;
			}
		}
		fprintf(ap, "%s=%s\n", lydx_get_cattr(node, "key"), lydx_get_cattr(node, "value"));
	}
	if (ap) {
		fclose(ap);
		fprintf(fp, " -e /run/containers/args/%s.env", name);
	}

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "network") {
		struct lyd_node *opt;
		const char *name;
		int first = 1;

		name = lydx_get_cattr(node, "name");
		fprintf(fp, " --net %s", name);
		LYX_LIST_FOR_EACH(lyd_child(node), opt, "option") {
			const char *option = lyd_get_value(opt);

			fprintf(fp, "%s%s", first ? ":" : ",", option);
			first = 0;
		}
	}

	if (lydx_is_enabled(cif, "host-network"))
		fprintf(fp, " --net host");

	if ((string = lydx_get_cattr(cif, "entrypoint")))
		fprintf(fp, " --entrypoint");

	fprintf(fp, " create %s %s", name, image);

 	if ((string = lydx_get_cattr(cif, "entrypoint")))
		fprintf(fp, " %s", string);

	fprintf(fp, "\n");
	fchmod(fileno(fp), 0700);
	fclose(fp);

	fp = fopenf("w", "/etc/finit.d/available/container:%s.conf", name);
	if (!fp) {
		ERROR("Failed creating container %s monitor", name);
		return SR_ERR_SYS;
	}

	restart_policy = lydx_get_cattr(cif, "restart-policy");
	if (restart_policy) {
		if (!strcmp(restart_policy, "no"))
			restart = "norestart";
		else if (!strcmp(restart_policy, "always"))
			restart = "restart:always";
		/* default is to restart up to 10 times */
	}

	fprintf(fp, "service name:container :%s log:prio:local1.err,tag:%s pid:!/run/container:%s.pid \\\n"
		"	[2345] %s %s <usr/container:%s> podman start -a %s -- Container %s\n",
		name, name, name, lydx_is_enabled(cif, "manual") ? "manual:yes" : "", restart,
		name, name, name);
	fclose(fp);

	if (systemf("initctl -nbq enable container:%s", name)) {
		ERROR("Failed enabling container %s monitor", name);
		return SR_ERR_SYS;
	}

	return 0;
}

static int del(const char *name)
{
	char fn[strlen(JOB_QUEUE) + strlen(name) + 5];

	/* Remove any pending download/create job first */
	snprintf(fn, sizeof(fn), "%s/%s.sh", JOB_QUEUE, name);
	erase(fn);
	snprintf(fn, sizeof(fn), "%s/%s.sh", INBOX_QUEUE, name);
	erase(fn);

	return systemf("container delete %s", name);
}

static int change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		  const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *diff, *cifs, *difs, *cif, *dif;
	sr_error_t       err = 0;
	sr_data_t       *cfg;

	switch (event) {
	case SR_EV_DONE:
		break;
	case SR_EV_CHANGE:
	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	err = sr_get_data(session, CFG_XPATH "//.", 0, 0, 0, &cfg);
	if (err)
		goto err_abandon;

	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;

	cifs = lydx_get_descendant(cfg->tree, "container", "container", NULL);
	difs = lydx_get_descendant(diff, "container", "container", NULL);

	/* find the modified one, delete or recreate only that */
	LYX_LIST_FOR_EACH(difs, dif, "container") {
		const char *name = lydx_get_cattr(dif, "name");

		ERROR("Change in container %s", name);
		if (lydx_get_op(dif) == LYDX_OP_DELETE) {
			ERROR("OP DELETE container %s", name);
			del(name);
			continue;
		}

		LYX_LIST_FOR_EACH(cifs, cif, "container") {
			const char *nm = lydx_get_cattr(cif, "name");

			ERROR("container %s vs %s", name, nm);
			if (strcmp(name, nm)) {
				ERROR("Skipping container %s", nm);
				continue;
			}

			if (!lydx_is_enabled(cif, "enabled")) {
				ERROR("container %s not enabled", nm);
				del(name);
			} else {
				ERROR("container %s enabled", nm);
				add(name, cif);
			}
			break;
		}
	}

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:

	return err;
}

void infix_containers_launch(void)
{
	struct dirent *d;
	DIR *dir;

	dir = opendir(INBOX_QUEUE);
	if (!dir) {
		ERROR("Cannot open %s to launch scripts.", INBOX_QUEUE);
		return;
	}

	while ((d = readdir(dir))) {
		char next[strlen(INBOX_QUEUE) + strlen(d->d_name) + 2];
		char curr[strlen(DONE_QUEUE) + strlen(d->d_name) + 2];

		snprintf(next, sizeof(next), "%s/%s", INBOX_QUEUE, d->d_name);
		snprintf(curr, sizeof(curr), "%s/%s", DONE_QUEUE, d->d_name);
		if (!systemf("cmp %s %s", curr, next)) {
			ERRNO("New job %s is already done, no changes, skipping.", next);
			systemf("initctl -nbq cond set container:$(basename %s .sh)", d->d_name);
			remove(next);
			continue;
		}

		if (movefile(next, JOB_QUEUE))
			ERRNO("Failed moving %s to job queue %s", next, JOB_QUEUE);
	}

	systemf("container -f volume prune");
}

int infix_containers_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, reqs);
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, MODULE, CFG_XPATH, 0, change, confd, &confd->sub);
	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
