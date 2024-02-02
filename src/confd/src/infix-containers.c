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
#define  CFG_XPATH    "/infix-containers:containers"
#define  INBOX_QUEUE  "/run/containers/inbox"
#define  JOB_QUEUE    "/run/containers/queue"
#define  ACTIVE_QUEUE "/var/lib/containers/active"
#define  LOGGER       "logger -t container -p local1.notice"

static const struct srx_module_requirement reqs[] = {
	{ .dir = YANG_PATH_, .name = MODULE, .rev = "2024-02-01" },
	{ NULL }
};

static int add(const char *name, struct lyd_node *cif)
{
	const char *image = lydx_get_cattr(cif, "image");
	const char *restart_policy, *string;
	struct lyd_node *node, *network;
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

	if (lydx_is_enabled(cif, "read-only"))
		fprintf(fp, " --read-only");

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "volume")
		fprintf(fp, " -v %s-%s:%s", name, lydx_get_cattr(node, "name"), lydx_get_cattr(node, "path"));

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "file") {
		const char *filenm = lydx_get_cattr(node, "name");
		const char *contdir = "/run/containers/files";
		char nm[strlen(filenm) + strlen(name) + 32];
		char buf[256];
		FILE *pp;

		snprintf(nm, sizeof(nm), "%s/%s-%s", contdir, name, filenm);
		snprintf(buf, sizeof(buf), "base64 -d > %s", nm);
		pp = popen(buf, "w");
		if (pp) {
			const char *data = lydx_get_cattr(node, "content");

			fputs(data, pp);
			pclose(pp);

			fprintf(fp, " -m type=bind,source=%s,destination=%s,readonly=true",
				nm, lydx_get_cattr(node, "path"));
		}
	}

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

	network = lydx_get_descendant(lyd_child(cif), "network", NULL);
	if (network) {
		if (lydx_is_enabled(network, "host")) {
			fprintf(fp, " --net host");
		} else {
			LYX_LIST_FOR_EACH(lyd_child(network), node, "interface") {
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

			LYX_LIST_FOR_EACH(lyd_child(network), node, "publish")
				fprintf(fp, " -p %s", lyd_get_value(node));
		}
	}

	if ((string = lydx_get_cattr(cif, "command")))
		fprintf(fp, " --entrypoint");

	fprintf(fp, " create %s %s", name, image);

 	if ((string = lydx_get_cattr(cif, "command")))
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
	const char *queue[] = {
		JOB_QUEUE,
		INBOX_QUEUE,
		ACTIVE_QUEUE,
	};
	int rc;

	/* Remove any pending download/create job first */
	for (size_t i = 0; i < NELEMS(queue); i++) {
		char fn[strlen(queue[i]) + strlen(name) + 5];

		snprintf(fn, sizeof(fn), "%s/%s.sh", queue[i], name);
		erase(fn);
	}

	systemf("initctl -nbq disable container:%s", name);
	rc = systemf("container delete %s", name);
	erasef("/etc/finit.d/available/container:%s.conf", name);

	return rc;
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

	cifs = lydx_get_descendant(cfg->tree, "containers", "container", NULL);
	difs = lydx_get_descendant(diff, "containers", "container", NULL);

	/* find the modified one, delete or recreate only that */
	LYX_LIST_FOR_EACH(difs, dif, "container") {
		const char *name = lydx_get_cattr(dif, "name");

		if (lydx_get_op(dif) == LYDX_OP_DELETE) {
			del(name);
			continue;
		}

		LYX_LIST_FOR_EACH(cifs, cif, "container") {
			const char *nm = lydx_get_cattr(cif, "name");

			if (strcmp(name, nm))
				continue;

			if (!lydx_is_enabled(cif, "enabled"))
				del(name);
			else
				add(name, cif);
			break;
		}
	}

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:

	return err;
}

static int action(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
		  const sr_val_t *input, const size_t input_cnt,
		  sr_event_t event, unsigned request_id,
		  sr_val_t **output, size_t *output_cnt,
		  void *priv)
{
	char buf[strlen(xpath) + 1];
	char *cmd, *name, *ptr;
	char quote;

	/* /infix-containers:containers/container[name='ntpd']/restart */
	strlcpy(buf, xpath, sizeof(buf));

	name = strstr(buf, "[name=");
	if (!name)
		return SR_ERR_INTERNAL;

	ptr   = &name[6];
	quote =  ptr[0];	/* Quote char to look for: ' or " */
	name  = &ptr[1];

	ptr = strchr(name, quote);
	if (!ptr)
		return SR_ERR_INTERNAL;
	*ptr++  = 0;

	cmd = strstr(ptr, "]/");
	if (!cmd)
		return SR_ERR_INTERNAL;
	cmd += 2;

	DEBUG("CALLING 'container %s %s' (xpath %s)", cmd, name, xpath);
	if (systemf("container %s %s", cmd, name))
		return SR_ERR_INTERNAL;

	return SR_ERR_OK;
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
		char curr[strlen(ACTIVE_QUEUE) + strlen(d->d_name) + 2];
		char next[strlen(INBOX_QUEUE) + strlen(d->d_name) + 2];

		if (d->d_name[0] == '.')
			continue;

		snprintf(curr, sizeof(curr), "%s/%s", ACTIVE_QUEUE, d->d_name);
		snprintf(next, sizeof(next), "%s/%s", INBOX_QUEUE, d->d_name);
		if (!systemf("cmp %s %s", curr, next)) {
			ERRNO("New job %s is already active, no changes, skipping.", next);
			systemf("initctl -nbq cond set container:$(basename %s .sh)", d->d_name);
			remove(next);
			continue;
		}

		if (movefile(next, JOB_QUEUE))
			ERRNO("Failed moving %s to job queue %s", next, JOB_QUEUE);
	}

	systemf("container volume prune -f");
}

int infix_containers_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, reqs);
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, MODULE, CFG_XPATH, 0, change, confd, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/start",   action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/stop",    action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/restart", action, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
