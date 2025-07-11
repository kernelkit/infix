/* SPDX-License-Identifier: BSD-3-Clause */
#include <srx/common.h>
#include <srx/lyx.h>
#include <openssl/bio.h>
#include <openssl/evp.h>
#include <sys/statvfs.h>

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

static int base64_decode_inplace(char *input, size_t input_len, size_t *output_len)
{
	BIO *bio, *b64;
	int decode_len;

	bio = BIO_new_mem_buf(input, input_len);
	if (!bio) {
		return -1;
	}

	b64 = BIO_new(BIO_f_base64());
	if (!b64) {
		BIO_free(bio);
		return -1;
	}

	BIO_set_flags(b64, BIO_FLAGS_BASE64_NO_NL);
	bio = BIO_push(b64, bio);

	/* Decode directly into the same buffer */
	decode_len = BIO_read(bio, input, input_len);
	BIO_free_all(bio);

	if (decode_len < 0) {
		return -1;
	}

	*output_len = decode_len;
	return 0;
}

static int infix_system_sw_install(sr_session_ctx_t *session, uint32_t sub_id,
				   const char *path, const sr_val_t *input,
				   const size_t input_cnt, sr_event_t event,
				   unsigned request_id, sr_val_t **output,
				   size_t *output_cnt, void *priv)
{
	sr_error_t srerr = SR_ERR_OK;
	GError *raucerr = NULL;
	RaucInstaller *rauc = NULL;
	char *install_source = NULL;

	const char *url = NULL;
	char *binary_data = NULL;
	size_t binary_len = 0;
	size_t decoded_len = 0;

	for (size_t i = 0; i < input_cnt; i++) {
		if (strcmp(input[i].xpath, "/infix-system:install-bundle/url") == 0) {
			if (input[i].type == SR_STRING_T) {
				url = input[i].data.string_val;
			}
		} else if (strcmp(input[i].xpath, "/infix-system:install-bundle/image") == 0) {
			if (input[i].type == SR_BINARY_T) {
				binary_data = (char *)input[i].data.binary_val;  // Cast away const for in-place decode
				binary_len = strlen(binary_data);  // Length of base64 string
			}
		}
	}

	if (url) {
		DEBUG("Installing from URL: %s", url);
		install_source = (char *)url;

	} else if (binary_data) {
		const char *temp_dir = "/tmp";
		char path[256];
		FILE *fp;

		DEBUG("Installing from uploaded binary data (%zu bytes base64)", binary_len);

		if (base64_decode_inplace(binary_data, binary_len, &decoded_len) != 0) {
			sr_session_set_netconf_error(session, "application", "invalid-value",
						     NULL, NULL, "Failed to decode base64 image data", 0);
			srerr = SR_ERR_INVAL_ARG;
			goto cleanup;
		}

		fmkpath(0775, "%s/images", temp_dir);
		snprintf(path, sizeof(path), "%s/images/install_bundle", temp_dir);

		fp = fopen(path, "wb");
		if (!fp) {
			ERROR("Could not open %s", path);
			return SR_ERR_NO_MEMORY;
		}

		fwrite(binary_data, sizeof(char), decoded_len, fp);
		fclose(fp);
		install_source = path;

	} else {
		ERROR("Unknown source");
		return 0;
	}

	rauc = infix_system_sw_new_rauc();
	if (!rauc) {
		sr_session_set_netconf_error(session, "application", "operation-failed",
					     NULL, NULL, "Failed to initialize RAUC installer", 0);
		srerr = SR_ERR_INTERNAL;
		goto cleanup;
	}

	rauc_installer_call_install_sync(rauc, install_source, NULL, &raucerr);
	if (raucerr) {
		sr_session_set_netconf_error(session, "application", "operation-failed",
					     NULL, NULL, raucerr->message, 0);
		g_error_free(raucerr);
		srerr = SR_ERR_OPERATION_FAILED;
	}

cleanup:
	if (rauc) {
		g_object_unref(rauc);
	}

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
