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
#define  LOGGER       "logger -t container -p local1.notice"

#define  MODULE       "infix-containers"
#define  CFG_XPATH    "/infix-containers:containers"

#define  _PATH_CONT   "/run/containers"
#define  _PATH_CLEAN  "/var/lib/containers/cleanup"

/*
 * Check if image is a local archive and return the offset to the file path.
 * Returns 0 if not a recognized local archive format.
 */
static int archive_offset(const char *image)
{
	static const struct {
		const char *prefix;
		int offset;
	} prefixes[] = {
		{ "docker-archive:", 15 },
		{ "oci-archive:",    12 },
		{ NULL, 0 }
	};
	int i;

	for (i = 0; prefixes[i].prefix; i++) {
		if (!strncmp(image, prefixes[i].prefix, prefixes[i].offset))
			return prefixes[i].offset;
	}

	return 0;
}

/*
 * Create a setup/create/upgrade script and instantiate a new instance
 * that Finit will start when all networking and other dependencies are
 * out of the way.  Finit calls the `/usr/sbin/container` wrapper script
 * in the pre: hook to fetch and create the container instance.
 *
 * The script we create here, on every boot, contains all information
 * needed to recreate and upgrade the user's container at runtime.
 */
static int add(const char *name, struct lyd_node *cif)
{
	const char *restart_policy, *string, *image;
	struct lyd_node *node, *nets, *caps;
	char script[strlen(name) + 5];
	FILE *fp, *ap;
	int offset;

	snprintf(script, sizeof(script), "%s.sh", name);
	fp = fopenf("w", "%s/%s", _PATH_CONT, script);
	if (!fp) {
		ERRNO("Failed creating container script %s/%s", _PATH_CONT, script);
		return SR_ERR_SYS;
	}

	/*
	 * Create /run/containers/<NAME>.sh it is used both for initial
	 * setup at creation/boot and for manual upgrade.  The delete
	 * command ensures any already running container is stopped and
	 * deleted so that it releases all claimed resources.
	 *
	 * The odd meta data is not used by the script itself, instead
	 * it is used by the /usr/sbin/container wrapper when upgrading
	 * a running container instance.
	 */
	image = lydx_get_cattr(cif, "image");
	fprintf(fp, "#!/bin/sh\n"
		"# meta-name: %s\n"
		"# meta-image: %s\n", name, image);

	offset = archive_offset(image);
	if (offset) {
		const char *path = image + offset;
		char sha256[65] = { 0 };
		FILE *pp;

		pp = popenf("r", "sha256sum %s | cut -f1 -d' '", path);
		if (pp) {
			if (fgets(sha256, sizeof(sha256), pp)) {
				chomp(sha256);
				fprintf(fp, "# meta-sha256: %s\n", sha256);
			}
			pclose(pp);
		}
	}

	fprintf(fp, "container --quiet delete %s >/dev/null\n"
		"container --quiet", name);

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "dns")
		fprintf(fp, " --dns %s", lyd_get_value(node));

	LYX_LIST_FOR_EACH(lyd_child(cif), node, "search")
		fprintf(fp, " --dns-search %s", lyd_get_value(node));

	if ((string = lydx_get_cattr(cif, "hostname"))) {
		char buf[65];

		if (hostnamefmt(&confd, string, buf, sizeof(buf), NULL, 0))
			ERRNO("%s: failed setting custom hostname", name);
		else
			fprintf(fp, " --hostname %s", buf);
	}

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
			const char *mode = lydx_get_cattr(node, "mode");
			const char *contdir = "/run/containers/files";
			mode_t file_mode = 0644;
			char cmd[256];
			int pos, fd;
			FILE *pp;

			if (mode) {
				unsigned long val;
				char *endptr;

				val = strtoul(mode, &endptr, 8);
				if (*endptr != '\0' || val > 07777) {
					ERROR("%s: invalid file mode '%s'", nm, mode);
					continue;
				}

				file_mode = (mode_t)val;
			}

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

			/*
			 * Always create with secure permissions, then immediately
			 * set final mode.  This takes care of both new files and
			 * updates to existing files atomically.
			 */
			fd = open(nm, O_CREAT | O_WRONLY | O_TRUNC, 0600);
			if (fd < 0) {
				ERRNO("%s: failed creating file %s", name, nm);
				continue;
			}

			/* Set final permissions */
			if (fchmod(fd, file_mode) < 0) {
				ERRNO("%s: failed setting file mode %s", nm, mode);
				close(fd);
				unlink(nm);
				continue;
			}
			close(fd);

			/* Now decode base64 content into the properly secured file */
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

	node = lydx_get_descendant(lyd_child(cif), "checksum", NULL);
	if (node) {
		if ((string = lydx_get_cattr(node, "md5")))
			fprintf(fp, " --checksum md5:%s", string);
		if ((string = lydx_get_cattr(node, "sha256")))
			fprintf(fp, " --checksum sha256:%s", string);
		if ((string = lydx_get_cattr(node, "sha512")))
			fprintf(fp, " --checksum sha512:%s", string);
	}

	fprintf(fp, " create %s %s", name, image);

 	if ((string = lydx_get_cattr(cif, "command")))
		fprintf(fp, " %s", string);

	fprintf(fp, "\n");
	fchmod(fileno(fp), 0700);
	fclose(fp);

	systemf("initctl -bnq touch container@%s.conf", name);
	systemf("initctl -bnq %s container@%s.conf", lydx_is_enabled(cif, "enabled")
		? "enable" : "disable", name);

	return 0;
}

/*
 * Remove setup/create/upgrade script and disable the currently running
 * instance.  The `/usr/sbin/container` wrapper script is called when
 * Finit removes the instance, and it does not need the file we erase.
 */
static int del(const char *name)
{
	char prune_dir[sizeof(_PATH_CLEAN) + strlen(name) + 3];
	char buf[256];
	FILE *pp;

	erasef("%s/%s.sh", _PATH_CONT, name);
	systemf("initctl -bnq disable container@%s.conf", name);

	/* Schedule a cleanup job for this container as soon as it has stopped */
	snprintf(prune_dir, sizeof(prune_dir), "%s/%s", _PATH_CLEAN, name);
	systemf("mkdir -p %s", prune_dir);

	/* Finit cleanup:script runs when container is deleted, it will remove any image by-ID */
	pp = popenf("r", "podman inspect %s 2>/dev/null | jq -r '.[].Id' 2>/dev/null", name);
	if (!pp) {
		/* Nothing to do, if we can't get the Id we cannot safely remove anything */
		ERROR("Cannot find any container instance named '%s' to delete", name);
		rmdir(prune_dir);
		return SR_ERR_OK;
	}

	if (fgets(buf, sizeof(buf), pp)) {
		chomp(buf);
		if (strlen(buf) > 2)
			touchf("%s/%s", prune_dir, buf);
	}

	pclose(pp);

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
		err = sr_get_data(session, CFG_XPATH "//.", 0, 0, 0, &cfg);
		if (err || !cfg)
			return SR_ERR_INTERNAL;

		cifs = lydx_get_descendant(cfg->tree, "containers", "container", NULL);
		LYX_LIST_FOR_EACH(cifs, cif, "container") {
			struct lyd_node *mount;
			LYX_LIST_FOR_EACH(lyd_child(cif), mount, "mount") {
				const char *src  = lydx_get_cattr(mount, "source");
				const char *id   = lydx_get_cattr(mount, "name");

				if (src && access(src, R_OK) != 0) {
				    	char errmsg[256];
				    	const char *reason = strerror(errno);
				    	snprintf(errmsg, sizeof(errmsg),
				             	"Container '%s': mount '%s' source file '%s' is invalid: %s",
				             	lydx_get_cattr(cif, "name"), id, src, reason);
				    	sr_session_set_error_message(session, errmsg);
				    	sr_release_data(cfg);
				    	return SR_ERR_VALIDATION_FAILED;
				}
			}
		}

		sr_release_data(cfg);
		return SR_ERR_OK;

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
			if (strcmp(name, lydx_get_cattr(cif, "name")))
				continue;

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

	DEBUG("RPC xpath %s, calling 'container %s %s'", xpath, cmd, name);
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
