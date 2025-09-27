/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/common.h>
#include <srx/lyx.h>

#include "core.h"
#include "rauc-installer.h"

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
	char *url = NULL;
	gboolean verify_certificate = TRUE;  /* default value */
	gboolean verify_compatible = TRUE;   /* default value */
	sr_error_t srerr = SR_ERR_OK;
	GError *raucerr = NULL;
	RaucInstaller *rauc;
	GVariantBuilder builder;
	GVariant *args;

	/* Parse input parameters */
	for (size_t i = 0; i < input_cnt; i++) {
		const sr_val_t *val = &input[i];
		const char *name;

		name = strrchr(val->xpath, '/');
		if (!name)
			continue;
		name++; /* skip '/' */

		if (strcmp(name, "url") == 0) {
			url = val->data.string_val;
		} else if (strcmp(name, "verify-certificate") == 0) {
			verify_certificate = val->data.bool_val;
		} else if (strcmp(name, "verify-compatible") == 0) {
			verify_compatible = val->data.bool_val;
		}
	}

	if (!url) {
		sr_session_set_netconf_error(session, "application", "missing-element",
					     NULL, NULL, "url parameter is required", 0);
		return SR_ERR_INVAL_ARG;
	}

	DEBUG("url:%s verify-certificate:%s verify-compatible:%s", url,
	      verify_certificate ? "true" : "false",
	      verify_compatible ? "true" : "false");

	rauc = infix_system_sw_new_rauc();
	if (!rauc)
		return SR_ERR_INTERNAL;

	/* Build args dictionary for InstallBundle method */
	g_variant_builder_init(&builder, G_VARIANT_TYPE("a{sv}"));
	g_variant_builder_add(&builder, "{sv}", "tls-no-verify",
			      g_variant_new_boolean(!verify_certificate));
	g_variant_builder_add(&builder, "{sv}", "ignore-compatible",
			      g_variant_new_boolean(!verify_compatible));
	args = g_variant_builder_end(&builder);

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

int infix_system_sw_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_RPC(confd->session, "/infix-system:install-bundle",
		     infix_system_sw_install, NULL, &confd->sub);
	REGISTER_RPC(confd->session, "/infix-system:set-boot-order",
		     infix_system_sw_set_boot_order, NULL, &confd->sub);

fail:
	return rc;
}
