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

