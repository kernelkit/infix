/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

#define  MODULE       "infix-containers"
#define  CFG_XPATH    "/infix-containers:containers"

#define  _PATH_CONT   "/run/containers"

/*
 * Write the container's configuration as JSON for ixcont, which builds
 * the OCI bundle from it in the Finit pre: hook of container@NAME, and
 * enable the instance.  Only the hostname is resolved here, it carries
 * the same format specifiers as the system hostname.
 */
static int add(const char *name, struct lyd_node *cif)
{
	struct lyd_node *dup = NULL, *node;
	const char *hostname;
	int enabled, manual;
	FILE *fp;

	if (lyd_dup_single(cif, NULL, LYD_DUP_RECURSIVE, &dup)) {
		ERROR("%s: failed duplicating container configuration", name);
		return SR_ERR_SYS;
	}

	hostname = lydx_get_cattr(dup, "hostname");
	if (hostname) {
		char buf[65];

		if (hostnamefmt(&confd, hostname, buf, sizeof(buf), NULL, 0))
			ERRNO("%s: failed setting custom hostname", name);
		else if ((node = lydx_get_child(dup, "hostname")))
			lyd_change_term(node, buf);
	}

	fp = fopenfp(0600, NULL, "%s/%s.json", _PATH_CONT, name);
	if (!fp) {
		ERRNO("Failed creating container configuration %s/%s.json", _PATH_CONT, name);
		lyd_free_tree(dup);
		return SR_ERR_SYS;
	}
	lyd_print_file(fp, dup, LYD_JSON, 0);
	fclose(fp);
	lyd_free_tree(dup);

	enabled = lydx_is_enabled(cif, "enabled");
	manual  = lydx_is_enabled(cif, "manual");
	if (enabled) {
		finit_enablef("container@%s", name);
		systemf("initctl -bnq cond %s container-%s", manual ? "clear" : "set", name);
		finit_reloadf("container@%s", name);
	} else {
		systemf("initctl -bnq cond clear container-%s", name);
		finit_disablef("container@%s", name);
	}

	return 0;
}

/*
 * Remove the configuration and disable the instance.  Finit stops a
 * running container and ixcont removes its state when it has exited,
 * the state of one already stopped is removed here.
 */
static int del(const char *name)
{
	erasef("%s/%s.json", _PATH_CONT, name);
	systemf("initctl -bnq cond clear container-%s", name);
	finit_disablef("container@%s", name);
	systemf("ixcont cleanup %s", name);

	return SR_ERR_OK;
}

int containers_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *cifs, *difs, *cif, *dif;
	sr_error_t       err = 0;

	if (diff && !lydx_get_xpathf(diff, CFG_XPATH))
		return SR_ERR_OK;

	switch (event) {
	case SR_EV_DONE:
		break;

	case SR_EV_CHANGE:
		cifs = lydx_get_descendant(config, "containers", "container", NULL);
		LYX_LIST_FOR_EACH(cifs, cif, "container") {
			const char *name = lydx_get_cattr(cif, "name");
			struct lyd_node *mount;

			/* The name is also the Finit condition usr/container-NAME */
			if (strchr(name, '.')) {
				sr_session_set_error_message(session, "Container '%s': names may not contain '.'", name);
				return SR_ERR_VALIDATION_FAILED;
			}
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
				    	return SR_ERR_VALIDATION_FAILED;
				}
			}
		}
		return SR_ERR_OK;

	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	cifs = lydx_get_descendant(config, "containers", "container", NULL);
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

	DEBUG("RPC xpath %s, calling 'ixcont %s %s'", xpath, cmd, name);
	if (systemf("ixcont %s %s >/dev/null", cmd, name))
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

	if (systemf("ixcont load %s %s 2>&1 | logger -t confd -p local1.err -I $PPID", uri, name))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

int containers_rpc_init(struct confd *confd)
{
	int rc;

	REGISTER_RPC(confd->session, CFG_XPATH "/container/start",   action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/stop",    action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/restart", action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, CFG_XPATH "/container/upgrade", action, NULL, &confd->sub);
	REGISTER_RPC(confd->session, "/infix-containers:oci-load", oci_load, NULL, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
