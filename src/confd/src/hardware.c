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

#define GPSD_CONF                "/etc/finit.d/available/gpsd.conf"
#define GPSD_CONF_NEXT           GPSD_CONF"+"

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
	struct lyd_node  *difs = NULL, *dif = NULL;
	int rc = SR_ERR_OK;
	int gps_changed = 0;
	int modem_changed = 0;
	int modem_rules_only = 0;

	if (!lydx_find_xpathf(diff, XPATH_BASE_)) {
		/* The generated udev rule embeds the bound interface
		 * name; regenerate rules (but leave services alone) when
		 * a wwan interface changes. */
		if (!modem_wwan_diff(diff))
			return SR_ERR_OK;
		modem_rules_only = 1;
	}

	/*
	 * The country code is one setting for every radio.  Apply the
	 * regulatory domain here, not from hostapd, so a radio without an
	 * interface is in the right domain too.
	 */
	if (event == SR_EV_DONE && lydx_get_xpathf(diff, XPATH_BASE_ "/infix-hardware:wifi/country-code"))
		systemf("iw reg set %s", wifi_country_code(config) ?: "00");

	difs = lydx_get_descendant(diff, "hardware", "component", NULL);
	LYX_LIST_FOR_EACH(difs, dif, "component") {
		struct lyd_node *state, *cif;
		const char *admin_state;
		const char *class, *name;

		name = lydx_get_cattr(dif, "name");

		/* Get the current config node for this component */
		cif = lydx_get_xpathf(config, "/hardware/component[name='%s']", name);
		if (!cif) {
			/* Component deleted: lock its USB port, modem services
			 * and udev rules are regenerated from the remaining
			 * config below. */
			class = lydx_get_cattr(dif, "class");
			if (event == SR_EV_DONE && class && !strcmp(class, "infix-hardware:usb") &&
			    usb_authorize(confd->root, name, 0)) {
				rc = SR_ERR_INTERNAL;
				goto err;
			}
			if (class && !strcmp(class, "infix-hardware:modem"))
				modem_changed = 1;
			if (class && !strcmp(class, "infix-hardware:gps"))
				gps_changed = 1;
			continue;
		}

		class = lydx_get_cattr(cif, "class");
		/* Handle USB components */
		if (!strcmp(class, "infix-hardware:usb")) {
			if (event != SR_EV_DONE)
				continue;

			state = lydx_get_child(cif, "state");
			admin_state = lydx_get_cattr(state, "admin-state");
			if (usb_authorize(confd->root, name,
					  admin_state && !strcmp(admin_state, "unlocked"))) {
				rc = SR_ERR_INTERNAL;
				goto err;
			}
		} else if (!strcmp(class, "infix-hardware:modem")) {
			/* Handled from full config after the loop; the
			 * services and udev rules span all modems. */
			modem_changed = 1;
		} else if (!strcmp(class, "infix-hardware:wifi")) {
			struct lyd_node **wifi_iface_list = NULL;
			struct lyd_node *interfaces_config;
			struct lyd_node *cwifi_radio;
			int wifi_iface_count = 0;

			switch (event) {
			case SR_EV_ABORT:
				continue;
			case SR_EV_CHANGE:
				break;
			default:
				continue;

			}

			cwifi_radio = lydx_get_child(cif, "wifi-radio");

			interfaces_config = lydx_get_descendant(config, "interfaces", "interface", NULL);

			wifi_find_interfaces_on_radio(interfaces_config, name,
						       &wifi_iface_list, &wifi_iface_count);

			if (wifi_iface_list) {
				if (wifi_iface_count) {
					/* Generate AP config (hostapd) for all APs on this radio */
					rc = wifi_gen_aps_on_radio(name, interfaces_config, cwifi_radio, config, confd);
					if (rc != SR_ERR_OK)
						ERROR("Failed to generate AP config for radio %s", name);
				}
				free(wifi_iface_list);
				wifi_iface_list = NULL;
				wifi_iface_count = 0;
			}
		} else if (!strcmp(class, "infix-hardware:gps")) {
			gps_changed = 1;
		}
	}

	if (gps_changed) {
		const char *gps_names[GPSD_MAX_DEVICES] = { NULL };
		struct lyd_node *cifs, *comp;
		int num_gps = 0;

		switch (event) {
		case SR_EV_UPDATE:
		case SR_EV_ENABLED:
		case SR_EV_RPC:
			break;

		case SR_EV_ABORT:
			unlink(GPSD_CONF_NEXT);
			break;

		case SR_EV_CHANGE:
			cifs = lydx_get_descendant(config, "hardware", "component", NULL);
			LYX_LIST_FOR_EACH(cifs, comp, "component") {
				const char *cls = lydx_get_cattr(comp, "class");

				if (strcmp(cls, "infix-hardware:gps"))
					continue;
				if (num_gps < GPSD_MAX_DEVICES)
					gps_names[num_gps] = lydx_get_cattr(comp, "name");
				else
					return SR_ERR_INTERNAL;
				num_gps++;
			}
			if (num_gps > 0) {
				FILE *fp;

				fp = fopenp(GPSD_CONF_NEXT, 0644, NULL);
				if (!fp) {
					ERRNO("Could not open " GPSD_CONF_NEXT);
					return SR_ERR_INTERNAL;
				}
				int i;

				fprintf(fp, "# Generated by confd, do not edit.\n");
				fprintf(fp, "service");
				fprintf(fp, " <!");
				for (i = 0; i < num_gps && i < GPSD_MAX_DEVICES; i++)
					fprintf(fp, "%sdev/%s", i ? "," : "", gps_names[i]);
				fprintf(fp, ">");
				fprintf(fp, " pid:!/run/gpsd.pid");
				fprintf(fp, " env:-/etc/default/gpsd \\\n");
				fprintf(fp, "\t[2345] gpsd -n");
				for (i = 0; i < num_gps && i < GPSD_MAX_DEVICES; i++)
					fprintf(fp, " /dev/%s", gps_names[i]);
				fprintf(fp, " -P /run/gpsd.pid -- GPS Daemon\n");
				fclose(fp);
			}
			break;

		case SR_EV_DONE:
			gps_gen_rules(config);
			if (fexist(GPSD_CONF_NEXT)) {
				if (rename(GPSD_CONF_NEXT, GPSD_CONF))
					ERRNO("Failed switching to new %s", GPSD_CONF);
				else
					finit_enable("gpsd");
			} else {
				unlink(GPSD_CONF);
				finit_disable("gpsd");
			}
			break;
		}
	}

	/*
	 * Modem components: regenerate udev anchor rules and the
	 * ModemManager environment from the full config, then drive the
	 * services.  modemd owns the bearer lifecycle and cleanly
	 * disconnects before ModemManager goes down, hence both services
	 * are driven together, from the aggregate state of all modems.
	 * On wwan-interface-only changes the rules are refreshed but the
	 * services are left alone; modemd is signaled by the interface
	 * dagger script.
	 */
	if ((modem_changed || modem_rules_only) && event == SR_EV_DONE) {
		int unlocked = modem_gen_rules(config);

		if (modem_changed) {
			if (unlocked > 0) {
				NOTE("Modem(s) enabled, starting modemd");
				finit_enable("modem-manager");
				finit_enable("modemd");
				systemf("initctl -bfqn restart modem-manager");
				systemf("initctl -bfqn restart modemd");
			} else {
				NOTE("No enabled modems, disabling modemd");
				systemf("initctl -bfqn stop modemd");
				finit_disable("modemd");
				systemf("initctl -bfqn stop modem-manager");
				finit_disable("modem-manager");
			}
		}
	}
err:

	return rc;
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
