/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include <stdlib.h>
#include <jansson.h>
#include <net/if.h>

#include <srx/common.h>
#include <srx/helpers.h>

json_t *json_get_output(const char *cmd)
{
	json_error_t j_err;
	json_t *j_root;
	FILE *proc;

	proc = popenf("re", "%s", cmd);
	if (!proc) {
		ERROR("Error, running command %s", cmd);
		return NULL;
	}

	j_root = json_loadf(proc, 0, &j_err);
	pclose(proc);
	if (!j_root) {
		ERROR("Error, parsing command JSON (%s)", cmd);
		return NULL;
	}

	return j_root;
}

int ip_link_check_group(const char *ifname, const char *group)
{
	char cmd[512] = {}; /* Size is arbitrary */
	json_t *j_iface;
	json_t *j_root;
	json_t *j_val;

	/* Check if it's a container interface */
	if (fexistf("/etc/cni/net.d/%s.conflist", ifname)) {
		/* Has it been handed over to another network namespace already? */
		if (!if_nametoindex(ifname)) {
			DEBUG("Interface %s currently in use by a container, skipping.", ifname);
			return 0;
		}
	}

	snprintf(cmd, sizeof(cmd), "ip -s -d -j link show dev %s 2>/dev/null", ifname);

	j_root = json_get_output(cmd);
	if (!j_root) {
		ERROR("Error, parsing ip-link JSON");
		return -1;
	}
	if (json_array_size(j_root) != 1) {
		ERROR("Error, expected JSON array of single iface");
		json_decref(j_root);
		return -1;
	}

	j_iface = json_array_get(j_root, 0);

	j_val = json_object_get(j_iface, "group");
	if (!json_is_string(j_val)) {
		ERROR("Error, expected a JSON string for 'group'");
		json_decref(j_root);
		return -1;
	}
	if (strcmp(json_string_value(j_val), group) == 0) {
		json_decref(j_root);
		return 1;
	}

	json_decref(j_root);

	return 0;
}
