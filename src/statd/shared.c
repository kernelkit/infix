#include <stdio.h>
#include <stdlib.h>
#include <jansson.h>

#include <srx/common.h>
#include <srx/helpers.h>

json_t *json_get_output(const char *cmd)
{
	json_error_t j_err;
	json_t *j_root;
	FILE *proc;

	proc = popenf("re", cmd);
	if (!proc) {
		ERROR("Error, running ip link command");
		return NULL;
	}

	j_root = json_loadf(proc, 0, &j_err);
	pclose(proc);
	if (!j_root) {
		ERROR("Error, parsing ip link JSON");
		return NULL;
	}

	if (!json_is_array(j_root)) {
		ERROR("Expected a JSON array from ip link");
		json_decref(j_root);
		return NULL;
	}

	return j_root;
}

