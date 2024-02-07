/* SPDX-License-Identifier: BSD-3-Clause */
#include <jansson.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_module.h>
#include <srx/srx_val.h>
#include <jansson.h>
#include <ftw.h>
#include <libgen.h>
#include <limits.h>

#include "core.h"

#define XPATH_BASE_ "/ietf-hardware:hardware"

static int dir_cb(const char *fpath, const struct stat *sb,
		  int typeflag, struct FTW *ftwbuf)
{
	char *filename;
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

static bool usb_authorize(struct json_t *root, const char *name, int enabled)
{
	json_t *usb_port, *usb_ports;
	int index;

	usb_ports = json_object_get(root, "usb-ports");
	if (!usb_ports) /* No Infix controlled USB ports is ok */
		return 0;

	json_array_foreach(usb_ports, index, usb_port) {
		struct json_t *jname, *jpath;
		const char *path;
		jname = json_object_get(usb_port, "name");
		if (!jname || !json_is_string(jname)) {
			ERROR("Did not find USB hardware port (name) for %s", name);
			continue;
		}
		if (!strcmp(name, json_string_value(jname))) {
			jpath = json_object_get(usb_port, "path");
			if (!jpath || !json_is_string(jpath)) {
				ERROR("Did not find USB hardware port (path) for %s", name);
				continue;
			}
			path = json_string_value(jpath);
			if (!enabled) {
				if (fexist(path)) {
					if (writedf(0, "w", "%s", path)) {
						ERROR("Failed to unauthorize %s", path);
						return 1;
					}
				}
			} else {
				char *rpath, *path_dup, *dir;
				path_dup = strdup(path);
				if (!path_dup) {
					ERROR("Failed to allocate memory.");
					return 1;
				}
				dir = dirname((char *)path_dup);
				rpath = realpath(dir, NULL);
				if (rpath) {
					nftw(rpath, dir_cb, 0, FTW_DEPTH | FTW_PHYS);
					free(rpath);
				}
				free(path_dup);
			}
		}
	}
	return 0;
}

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
			break;
		}
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

static int change_hardware(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
			   const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	struct lyd_node *diff, *difs = NULL, *dif = NULL, *cifs = NULL, *cif = NULL;
	sr_data_t *cfg;
	int rc = SR_ERR_OK;
	struct confd *confd = (struct confd *)priv;

	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	rc = sr_get_data(session, XPATH_BASE_ "//.", 0, 0, 0, &cfg);
	if (rc)
		goto err;

	rc = srx_get_diff(session, &diff);
	if (rc)
		goto err_release_data;

	cifs = lydx_get_descendant(cfg->tree, "hardware", "component", NULL);
	difs = lydx_get_descendant(diff, "hardware", "component", NULL);

	LYX_LIST_FOR_EACH(difs, dif, "component") {
		enum lydx_op op;
		struct lyd_node *state;
		const char *admin_state;
		const char *class, *name;

		op = lydx_get_op(dif);
		name = lydx_get_cattr(dif, "name");
		if (op == LYDX_OP_DELETE) {
			if (usb_authorize(confd->root, name, 0)) {
				rc = SR_ERR_INTERNAL;
				goto err_release_diff;
			}
			continue;
		}

		LYX_LIST_FOR_EACH(cifs, cif, "component") {
			if (strcmp(name, lydx_get_cattr(cif, "name")))
				continue;

			class = lydx_get_cattr(cif, "class");
			if (strcmp(class, "infix-hardware:usb")) {
				continue;
			}
			state = lydx_get_child(dif, "state");
			admin_state = lydx_get_cattr(state, "admin-state");
			if (usb_authorize(confd->root, name, !strcmp(admin_state, "unlocked"))) {
				rc = SR_ERR_INTERNAL;
				goto err_release_diff;
			}
		}
	}

err_release_diff:
	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err:
	return rc;
}
int ietf_hardware_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_CHANGE(confd->session, "ietf-hardware", XPATH_BASE_, 0, change_hardware, confd, &confd->sub);
	REGISTER_CHANGE(confd->cand, "ietf-hardware", XPATH_BASE_,
			SR_SUBSCR_UPDATE, hardware_cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("Init hardware failed: %s", sr_strerror(rc));
	return rc;
}
