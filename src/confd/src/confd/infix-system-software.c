/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"
#include "../lib/common.h"
#include "../lib/lyx.h"
#include "rauc-installer.h"

#include <sysrepo/error_format.h>

#define SW_STATE_PATH_    "/ietf-system:system-state/infix-system:software"

static const struct lys_module *infix_system;

static RaucInstaller *infix_system_sw_new_rauc(void)
{
	RaucInstaller *rauc;
	GError *raucerr = NULL;

	rauc = rauc_installer_proxy_new_for_bus_sync(G_BUS_TYPE_SYSTEM, G_DBUS_PROXY_FLAGS_NONE,
						     "de.pengutronix.rauc", "/", NULL, &raucerr);
	if (raucerr) {
		ERROR("Unable to connect to RAUC: %s", raucerr->message);
		g_error_free(raucerr);
		return NULL;
	}

	return rauc;
}

static sr_error_t infix_system_sw_state_fill_installer(struct lyd_node *inst,
						       RaucInstaller *rauc)
{
	struct lyd_node *progress;
	GVariant *props, *val;
	const char *strval;

	strval = rauc_installer_get_operation(rauc);
	if (strval && strval[0] &&
	    lyd_new_term(inst, NULL, "operation", strval, 0, NULL))
		return SR_ERR_INTERNAL;

	strval = rauc_installer_get_last_error(rauc);
	if (strval && strval[0] &&
	    lyd_new_term(inst, NULL, "last-error", strval, 0, NULL))
		return SR_ERR_INTERNAL;

	props = rauc_installer_get_progress(rauc);
	if (props) {
		if (lyd_new_inner(inst, NULL, "progress", 0, &progress))
			return SR_ERR_INTERNAL;

		g_variant_get(props, "(@isi)", &val, &strval, NULL);

		if (lyd_new_term(progress, NULL, "percentage",
				 g_variant_print(val, FALSE), 0, NULL))
			return SR_ERR_INTERNAL;

		if (strval && strval[0] &&
		    lyd_new_term(progress, NULL, "message", strval, 0, NULL))
			return SR_ERR_INTERNAL;
	}
	return SR_ERR_OK;
}

static sr_error_t infix_system_sw_state_fill_slot(struct lyd_node *slot,
						  GVariant *props)
{
	static const char *strprops[] = {
		"bootname",
		"class",
		"state",
		"sha256",
		NULL
	};
	struct lyd_node *section;
	const char **strprop;
	const char *strval;
	GVariant *val;

	for (strprop = strprops; *strprop; strprop++) {
		if (g_variant_lookup(props, *strprop, "s", &strval) &&
		    lyd_new_term(slot, NULL, *strprop, strval, 0, NULL))
			return SR_ERR_INTERNAL;
	}

	if (g_variant_lookup(props, "size", "@t", &val) &&
	    lyd_new_term(slot, NULL, "size", g_variant_print(val, FALSE), 0, NULL))
		return SR_ERR_INTERNAL;


	if (lyd_new_inner(slot, NULL, "bundle", 0, &section))
		return SR_ERR_INTERNAL;

	if (g_variant_lookup(props, "bundle.compatible", "s", &strval) &&
	    lyd_new_term(section, NULL, "compatible", strval, 0, NULL))
		return SR_ERR_INTERNAL;

	if (g_variant_lookup(props, "bundle.version", "s", &strval) &&
	    lyd_new_term(section, NULL, "version", strval, 0, NULL))
		return SR_ERR_INTERNAL;


	if (lyd_new_inner(slot, NULL, "installed", 0, &section))
		return SR_ERR_INTERNAL;

	if (g_variant_lookup(props, "installed.timestamp", "s", &strval) &&
	    lyd_new_term(section, NULL, "datetime", strval, 0, NULL))
		return SR_ERR_INTERNAL;

	if (g_variant_lookup(props, "installed.count", "@u", &val) &&
	    lyd_new_term(section, NULL, "count", g_variant_print(val, FALSE), 0, NULL))
		return SR_ERR_INTERNAL;


	if (lyd_new_inner(slot, NULL, "activated", 0, &section))
		return SR_ERR_INTERNAL;

	if (g_variant_lookup(props, "activated.timestamp", "s", &strval) &&
	    lyd_new_term(section, NULL, "datetime", strval, 0, NULL))
		return SR_ERR_INTERNAL;

	if (g_variant_lookup(props, "activated.count", "@u", &val) &&
	    lyd_new_term(section, NULL, "count", g_variant_print(val, FALSE), 0, NULL))
		return SR_ERR_INTERNAL;

	return SR_ERR_OK;
}

static sr_error_t infix_system_sw_state_fill(struct lyd_node *sw,
					     RaucInstaller *rauc)
{
	struct lyd_node *slot, *inst;
	GVariant *slots, *props;
	GError *raucerr = NULL;
	GVariantIter slotiter;
	sr_error_t srerr;
	char *slotname;
	const char *val;

	val = rauc_installer_get_compatible(rauc);
	if (val && val[0] && lyd_new_term(sw, NULL, "compatible", val, 0, NULL))
		return SR_ERR_INTERNAL;

	val = rauc_installer_get_variant(rauc);
	if (val && val[0] && lyd_new_term(sw, NULL, "variant", val, 0, NULL))
		return SR_ERR_INTERNAL;

	val = rauc_installer_get_boot_slot(rauc);
	if (val && val[0] && lyd_new_term(sw, NULL, "booted", val, 0, NULL))
		return SR_ERR_INTERNAL;

	if (lyd_new_inner(sw, NULL, "installer", 0, &inst))
		return SR_ERR_INTERNAL;

	srerr = infix_system_sw_state_fill_installer(inst, rauc);
	if (srerr)
		return srerr;

	if (!rauc_installer_call_get_slot_status_sync(rauc, &slots, NULL, &raucerr)) {
		/* Slot status is not available while an installation
		 * is in progress, so we don't consider that an error.
		 */
		g_error_free(raucerr);
		return SR_ERR_OK;
	}

	for (g_variant_iter_init(&slotiter, slots);
	     g_variant_iter_next(&slotiter, "(s@a{sv})", &slotname, &props);) {
		if (lyd_new_list(sw, NULL, "slot", 0, &slot, slotname))
			return SR_ERR_INTERNAL;

		srerr = infix_system_sw_state_fill_slot(slot, props);
		if (srerr)
			break;
	}

	return srerr;
}

static int infix_system_sw_state(sr_session_ctx_t *session, uint32_t sub_id,
				 const char *module, const char *path,
				 const char *request_path, uint32_t request_id,
				 struct lyd_node **parent, void *priv)
{
	sr_error_t srerr = SR_ERR_INTERNAL;
	RaucInstaller *rauc;
	struct lyd_node *sw;

	DEBUG("%s", path);

	rauc = infix_system_sw_new_rauc();
	if (!rauc)
		return SR_ERR_INTERNAL;

	if (lyd_new_inner(*parent, infix_system, "software", 0, &sw))
		goto out;

	srerr = infix_system_sw_state_fill(sw, rauc);
out:
	g_object_unref(rauc);
	return srerr;
}


static int infix_system_sw_install(sr_session_ctx_t *session, uint32_t sub_id,
				   const char *path, const sr_val_t *input,
				   const size_t input_cnt, sr_event_t event,
				   unsigned request_id, sr_val_t **output,
				   size_t *output_cnt, void *priv)
{
	char *url = input->data.string_val;
	sr_error_t srerr = SR_ERR_OK;
	GError *raucerr = NULL;
	RaucInstaller *rauc;

	DEBUG("url:%s", url);

	rauc = infix_system_sw_new_rauc();
	if (!rauc)
		return SR_ERR_INTERNAL;

	rauc_installer_call_install_sync(rauc, url, NULL, &raucerr);
	if (raucerr) {
		sr_session_set_netconf_error(session, "application", "operation-failed",
					     NULL, NULL, raucerr->message, 0);
		g_error_free(raucerr);
		srerr = SR_ERR_OPERATION_FAILED;
	}

	g_object_unref(rauc);
	return srerr;
}

int infix_system_sw_init(struct confd *confd)
{
	const struct ly_ctx *ly;
	int rc = 0;

	ly = sr_acquire_context(sr_session_get_connection(confd->session));
	infix_system = ly_ctx_get_module_implemented(ly, "infix-system");
	sr_release_context(sr_session_get_connection(confd->session));
	if (!infix_system) {
		ERROR("infix-system module not found");
		rc = -ENOENT;
		goto fail;
	}

	REGISTER_OPER(confd->session, "ietf-system", SW_STATE_PATH_,
		      infix_system_sw_state, NULL, 0, &confd->sub);

	REGISTER_RPC(confd->session, "/infix-system:install-bundle",
		     infix_system_sw_install, NULL, &confd->sub);

fail:
	return rc;
}
