/* SPDX-License-Identifier: BSD-3-Clause */
#include <fnmatch.h>
#include <jansson.h>
#include <unistd.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include "hardware.h"
#include "dagger.h"

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
			goto out_free_name;
		}
	}

	if (!fnmatch("radio+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:wifi";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}

	if (!fnmatch("gps+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:gps";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}

	if (!fnmatch("modem+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:modem";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
	}

	if (!fnmatch("sim+([0-9])", name, FNM_EXTMATCH)) {
		inferred.data.string_val = "infix-hardware:sim";
		err = srx_set_item(session, &inferred, 0, "%s/class", xpath);
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

int hardware_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
		    sr_event_t event, struct confd *confd)
{
	struct lyd_node *difs, *dif;
	bool modem = false;
	bool gps = false;
	int rc;

	wifi_country_change(config, diff, event);

	difs = lydx_get_descendant(diff, "hardware", "component", NULL);
	LYX_LIST_FOR_EACH(difs, dif, "component") {
		const char *name = lydx_get_cattr(dif, "name");
		struct lyd_node *cif;
		const char *class;

		/* A deleted component is only in the diff */
		cif = lydx_get_xpathf(config, "/hardware/component[name='%s']", name);
		class = lydx_get_cattr(cif ?: dif, "class");
		if (!class)
			continue;

		rc = SR_ERR_OK;
		if (!strcmp(class, "infix-hardware:usb"))
			rc = usb_change(cif, name, event, confd);
		else if (!strcmp(class, "infix-hardware:wifi"))
			rc = wifi_radio_change(config, cif, event, confd);
		else if (!strcmp(class, "infix-hardware:modem"))
			modem = true;
		else if (!strcmp(class, "infix-hardware:gps"))
			gps = true;
		if (rc)
			return rc;
	}

	/* GPS and modem services and udev rules span all components */
	if (gps) {
		rc = gps_change(config, event);
		if (rc)
			return rc;
	}

	/* The modem udev rules name the wwan interface, a change to one
	 * regenerates the rules but leaves the services alone. */
	if (modem || modem_wwan_diff(diff))
		return modem_change(config, event, modem);

	return SR_ERR_OK;
}

int hardware_candidate_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_CHANGE(confd->cand, "ietf-hardware", XPATH_BASE_,
			SR_SUBSCR_UPDATE, hardware_cand, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("Init hardware failed: %s", sr_strerror(rc));
	return rc;
}
