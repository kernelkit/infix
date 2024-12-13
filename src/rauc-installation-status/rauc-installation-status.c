/* SPDX-License-Identifier: BSD-3-Clause */
#include <errno.h>
#include <jansson.h>
#include <stdio.h>

#include "rauc-installer.h"

static RaucInstaller *infix_system_sw_new_rauc(void) {
	RaucInstaller *rauc;
	GError *raucerr = NULL;

	rauc = rauc_installer_proxy_new_for_bus_sync(G_BUS_TYPE_SYSTEM, G_DBUS_PROXY_FLAGS_NONE,
						     "de.pengutronix.rauc", "/", NULL, &raucerr);
	if (raucerr) {
		fprintf(stderr, "Unable to connect to RAUC: %s\n", raucerr->message);
		g_error_free(raucerr);
		return NULL;
	}

	return rauc;
}


int main(int argc, char **argv)
{
	json_t *json, *progress;
	RaucInstaller *rauc;
	const char *strval;
	GVariant *props;
	char *output;


	rauc = infix_system_sw_new_rauc();
	if (!rauc)
		return 1;

	json = json_object();
	if (!json)
		return 1;

	strval = rauc_installer_get_operation(rauc);
 	if (strval && strval[0])
		json_object_set_new(json, "operation", json_string(strval));

	strval = rauc_installer_get_last_error(rauc);
	if (strval && strval[0])
		json_object_set_new(json, "last-error", json_string(strval));
	props = rauc_installer_get_progress(rauc);
	if(props) {
		GVariant *val;
		progress = json_object();
		g_variant_get(props, "(@isi)", &val, &strval, NULL);
		json_object_set_new(progress, "percentage",  json_string(g_variant_print(val, FALSE)));
		json_object_set_new(progress, "message", json_string(strval));
		json_object_set_new(json, "progress", progress);
	}
	output = json_dumps(json, 0);
	printf("%s", output);
	free(output);
	json_decref(json);

	return 0;
}
