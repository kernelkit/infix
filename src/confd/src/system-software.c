/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/common.h>
#include <srx/lyx.h>

#include <jansson.h>

#include "core.h"
#include "rauc-installer.h"

#define UPDATE_STATE "/run/software-update.json"

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
	GVariant *args;

	DEBUG("url:%s", url);

	rauc = infix_system_sw_new_rauc();
	if (!rauc)
		return SR_ERR_INTERNAL;

	/* Empty args dictionary for InstallBundle method, for now. */
	args = g_variant_new("a{sv}", NULL);

	rauc_installer_call_install_bundle_sync(rauc, url, args, NULL, &raucerr);
	if (raucerr) {
		sr_session_set_netconf_error(session, "application", "operation-failed",
					     NULL, NULL, raucerr->message, 0);
		g_error_free(raucerr);
		srerr = SR_ERR_OPERATION_FAILED;
	}

	g_object_unref(rauc);
	return srerr;
}

/*
  boot order can only be: primary, secondary and net, limited by model
 */
static int infix_system_sw_set_boot_order(sr_session_ctx_t *session, uint32_t sub_id,
					  const char *path, const sr_val_t *input,
					  const size_t input_cnt, sr_event_t event,
					  unsigned request_id, sr_val_t **output,
					  size_t *output_cnt, void *priv) {
	char boot_order[23] = "";
	for (size_t i = 0; i < input_cnt; i++) {
		const sr_val_t *val = &input[i];

		if (i != 0)
			 strlcat(boot_order, " ", sizeof(boot_order));
		 strlcat(boot_order, val->data.string_val, sizeof(boot_order));
	 }

	 if (fexist("/sys/firmware/devicetree/base/chosen/u-boot,version")) {
		 if (systemf("fw_setenv BOOT_ORDER %s", boot_order)) {
			 ERROR("Set-boot-order: Failed to set boot order in U-Boot");
			 return SR_ERR_INTERNAL;
		 }
	 } else if (fexist("/mnt/aux/grub/grubenv")) {
		 if (systemf("grub-editenv /mnt/aux/grub/grubenv set ORDER=\"%s\"", boot_order)) {
			 ERROR("Set-boot-order: Failed to set boot order in Grub");
			 return SR_ERR_INTERNAL;
		 }
	 } else {
		 ERROR("No supported boot loader found");
		 return SR_ERR_UNSUPPORTED;
	 }

	return SR_ERR_OK;
}

static int rpc_failed(sr_session_ctx_t *session, const char *msg)
{
	sr_session_set_netconf_error(session, "application", "operation-failed",
				     NULL, NULL, msg, 0);
	return SR_ERR_OPERATION_FAILED;
}

/* Append output leaf PATH/LEAF from the state file, skipped when absent. */
static int add_output(sr_val_t **output, size_t *cnt, const char *path,
		      const char *leaf, json_t *val)
{
	sr_val_t *v;

	if (!val || json_is_null(val))
		return 0;
	if (sr_realloc_values(*cnt, *cnt + 1, output))
		return -1;

	v = &(*output)[(*cnt)++];
	if (sr_val_build_xpath(v, "%s/%s", path, leaf))
		return -1;
	if (json_is_boolean(val)) {
		v->type = SR_BOOL_T;
		v->data.bool_val = json_is_true(val);
		return 0;
	}

	return sr_val_set_str_data(v, SR_STRING_T, json_string_value(val)) ? -1 : 0;
}

/*
 * Run the scheduled check now.  The script records the outcome in the
 * state file yanger serves, the reply repeats the parts a caller acts on.
 */
static int infix_system_sw_check_update(sr_session_ctx_t *session, uint32_t sub_id,
					const char *path, const sr_val_t *input,
					const size_t input_cnt, sr_event_t event,
					unsigned request_id, sr_val_t **output,
					size_t *output_cnt, void *priv)
{
	const char *leaves[] = { "latest", "available", "release-url", "bundle-url" };
	json_error_t jerr;
	json_t *state;
	size_t cnt = 0;
	int rc;

	rc = systemf("/usr/sbin/check-update -s");
	switch (rc) {
	case 0:
		break;
	case 2:
		return rpc_failed(session, "Could not read the release feed, see the log for details");
	case 3:
		return rpc_failed(session, "No update source configured, set system software update-url");
	default:
		return rpc_failed(session, "Update check failed, see the log for details");
	}

	state = json_load_file(UPDATE_STATE, 0, &jerr);
	if (!state)
		return rpc_failed(session, "Update check left no result");

	for (size_t i = 0; i < NELEMS(leaves); i++) {
		if (add_output(output, &cnt, path, leaves[i], json_object_get(state, leaves[i]))) {
			sr_free_values(*output, cnt);
			*output = NULL;
			json_decref(state);
			return SR_ERR_NO_MEMORY;
		}
	}
	json_decref(state);

	*output_cnt = cnt;
	return SR_ERR_OK;
}

int system_sw_rpc_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_RPC(confd->session, "/infix-system:install-bundle",
		     infix_system_sw_install, NULL, &confd->sub);
	REGISTER_RPC(confd->session, "/infix-system:set-boot-order",
		     infix_system_sw_set_boot_order, NULL, &confd->sub);
	REGISTER_RPC(confd->session, "/infix-system:check-update",
		     infix_system_sw_check_update, NULL, &confd->sub);

fail:
	return rc;
}
