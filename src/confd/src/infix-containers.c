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
#include <srx/srx_val.h>

#include "core.h"
#define  ARPING_MSEC  1000
#define  MODULE       "infix-containers"
#define  CFG_XPATH    "/infix-containers:containers"
#define  INBOX_QUEUE  "/run/containers/inbox"
#define  JOB_QUEUE    "/run/containers/queue"
#define  ACTIVE_QUEUE "/var/lib/containers/active"
#define  LOGGER       "logger -t container -p local1.notice"

static int add(const char *name, struct lyd_node *cif)
{
	const char *image = lydx_get_cattr(cif, "image");
	const char *restart_policy, *string;
	struct lyd_node *node, *nets, *caps;
	FILE *fp, *ap;

	fp = fopenf("w", "%s/S01-%s.sh", INBOX_QUEUE, name);
	if (!fp) {
		ERRNO("Failed adding job S01-%s.sh to job queue" INBOX_QUEUE, name);
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

	if (lydx_is_enabled(cif, "privileged"))
		fprintf(fp, " --privileged");

	caps = lydx_get_descendant(lyd_child(cif), "capabilities", NULL);
	if (caps) {
		LYX_LIST_FOR_EACH(lyd_child(caps), node, "add")
			fprintf(fp, " --cap-add %s", lyd_get_value(node));
		LYX_LIST_FOR_EACH(lyd_child(caps), node, "drop")
			fprintf(fp, " --cap-drop %s", lyd_get_value(node));
	}

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "volume")
		fprintf(fp, " -v %s-%s:%s", name, lydx_get_cattr(node, "name"),
			lydx_get_cattr(node, "target"));

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "mount") {
		const char *dst = lydx_get_cattr(node, "target");
		const char *data = lydx_get_cattr(node, "content");
		const char *src = lydx_get_cattr(node, "source");
		const char *type = lydx_get_cattr(node, "type");
		const char *id = lydx_get_cattr(node, "name");
		int ro = lydx_get_bool(node, "read-only");
		char nm[strlen(id) + strlen(name) + 32];

		/* Content mount: create a unique file with 'content' and bind mount */
		if (data) {
			const char *contdir = "/run/containers/files";
			char cmd[256];
			FILE *pp;
			int pos;

			/*
			 * prefix file name with container name, shared namespace,
			 * and replace any slashes with hyphens.
			 */
			pos = snprintf(nm, sizeof(nm), "%s/%s-", contdir, name);
			strlcat(nm, id, sizeof(nm));
			for (int i = pos; nm[i] && i < (int)sizeof(nm); i++) {
				if (nm[i] == '/')
					nm[i] = '-';
			}

			snprintf(cmd, sizeof(cmd), "base64 -d > %s", nm);
			pp = popen(cmd, "w");
			if (!pp || fputs(data, pp) < 0) {
				ERRNO("%s: failed decoding %s base64 'content'", name, id);
				if (pp)
					pclose(pp);
				continue;
			}

			pclose(pp);
			type = "bind"; /* discard any configured setting */
			src = nm;      /* discard any source, not used for content mounts */
		}

		fprintf(fp, " -m type=%s,src=%s,dst=%s,readonly=%s",
			type, src, dst, ro ? "true" : "false");
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

	nets = lydx_get_descendant(lyd_child(cif), "network", NULL);
	if (nets) {
		if (lydx_is_enabled(nets, "host")) {
			fprintf(fp, " --net host");
		} else {
			LYX_LIST_FOR_EACH(lyd_child(nets), node, "interface") {
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

			LYX_LIST_FOR_EACH(lyd_child(nets), node, "publish")
				fprintf(fp, " -p %s", lyd_get_value(node));
		}
	}

	restart_policy = lydx_get_cattr(cif, "restart-policy");
	if (restart_policy) {
		if (!strcmp(restart_policy, "never"))
			fprintf(fp, " -r no"); /* for docker compat */
		else if (!strcmp(restart_policy, "always"))
			fprintf(fp, " -r always");
		else
			fprintf(fp, " -r on-failure:10");
	}

	if (lydx_is_enabled(cif, "manual"))
		fprintf(fp, " --manual");

	fprintf(fp, " create %s %s", name, image);

 	if ((string = lydx_get_cattr(cif, "command")))
		fprintf(fp, " %s", string);

	fprintf(fp, "\n");
	fchmod(fileno(fp), 0700);
	fclose(fp);

	systemf("initctl -bnq enable container@%s.conf", name);

	return 0;
}

static int del(const char *name)
{
	const char *queue[] = {
		JOB_QUEUE,
		INBOX_QUEUE,
		ACTIVE_QUEUE,
	};
	FILE *fp;

	/* Remove any pending download/create job first */
	for (size_t i = 0; i < NELEMS(queue); i++) {
		char fn[strlen(queue[i]) + strlen(name) + 5];

		snprintf(fn, sizeof(fn), "%s/%s.sh", queue[i], name);
		erase(fn);
	}

	/* Disable service and schedule for deletion. */
	systemf("initctl -bnq disable container@%s.conf", name);

	fp = fopenf("w", "%s/K01-%s.sh", INBOX_QUEUE, name);
	if (!fp) {
		ERRNO("Failed adding job 00-delete-%s.sh to job queue" INBOX_QUEUE, name);
		return SR_ERR_SYS;
	}

	fprintf(fp, "#!/bin/sh\n"
		"container delete %s\n", name);
	fchmod(fileno(fp), 0700);
	fclose(fp);

	return SR_ERR_OK;
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
	if (err || !cfg)
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

static int oci_load(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
		    const sr_val_t *input, const size_t input_cnt, sr_event_t event,
		    unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	char *uri, *name = "";

	uri = input[0].data.string_val; /* mandatory */
	if (input_cnt > 1)
		name = input[1].data.string_val;

	if (systemf("container load %s %s 2>&1 | logger -t confd -p local1.err -I $PPID", uri, name))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

static int is_active(sr_session_ctx_t *session, const char *name)
{
	return srx_enabled(session, CFG_XPATH "/container[name='%s']/enabled", name);
}

static int is_manual(sr_session_ctx_t *session, const char *name)
{
	return srx_enabled(session, CFG_XPATH "/container[name='%s']/manual", name);
}

/*
 * When container configurations are not saved to startup-config and the
 * user reboot the system (or lose power) we will have lingering active
 * containers cached on persistent storage.
 *
 * This function runs every time a configuration is applied to clean up
 * any lingering active jobs to prevent false matches in the cmp magic
 * in the below post-hook.
 */
static void cleanup(sr_session_ctx_t *session, struct confd *confd)
{
	struct dirent *d;
	DIR *dir;

	dir = opendir(ACTIVE_QUEUE);
	if (!dir)
		return;

	while ((d = readdir(dir))) {
		char name[strlen(ACTIVE_QUEUE) + strlen(d->d_name) + 2];
		char *ptr;

		if (d->d_name[0] == '.')
			continue;

		strlcpy(name, d->d_name, sizeof(name));
		ptr = strstr(name, ".sh");
		if (!ptr)
			continue; /* odd, non-script file? */
		*ptr = 0;

		if (is_active(session, name))
			continue;

		/* Not found in running-config, remove stale cache. */
		snprintf(name, sizeof(name), "%s/%s", ACTIVE_QUEUE, d->d_name);
		if (erase(name))
			ERRNO("Failed removing stale container job %s", name);
	}

	closedir(dir);
}

/*
 * Containers depend on a lot of other system resources being properly
 * set up, e.g., networking, which is run by dagger.  So we need to wait
 * for all that before we can launch new, or modified, containers.  The
 * latter is the tricky part.
 *
 * By default, containers get a writable layer which is preserved across
 * restarts/reboots of container or host -- provided we don't recreate
 * them on a reboot.  Hence the cmp magic below: we check if the command
 * to create a container is the same as what is already activated, if it
 * is already activated we know 'podman create' has done its thing and
 * we can safely start the container.
 */
void infix_containers_post_hook(sr_session_ctx_t *session, struct confd *confd)
{
	struct dirent *d;
	DIR *dir;

	cleanup(session, confd);

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
		if (!systemf("cmp %s %s >/dev/null 2>&1", curr, next)) {
			char name[strlen(d->d_name) + 1];
			char *ptr;

			strlcpy(name, d->d_name, sizeof(name));
			ptr = strstr(name, ".sh");
			if (ptr) {
				*ptr = 0;

				/* New job is already active, no changes, skipping ... */
				if (!is_manual(session, name))
					systemf("initctl -bnq cond set container:%s", name);
			}
			remove(next);
			continue;
		}

		if (movefile(next, JOB_QUEUE))
			ERRNO("Failed moving %s to job queue %s", next, JOB_QUEUE);
	}

	closedir(dir);
	systemf("container volume prune -f >/dev/null 2>&1");
}

int infix_containers_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->session, MODULE, CFG_XPATH, 0, change, confd, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/start",   action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/stop",    action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/restart", action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, "/infix-containers:oci-load", oci_load, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
