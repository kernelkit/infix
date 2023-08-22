/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <libyang/libyang.h>

#include "common.h"
#include "srx_module.h"

sr_error_t srx_require_module(sr_conn_ctx_t *conn, const struct srx_module_requirement *mr)
{
	sr_error_t err = SR_ERR_OK;
	char *path;
	int len;

	len = asprintf(&path, "%s%s%s.yang", mr->name, mr->rev ? "@" : "", mr->rev ? : "");
	if (len == -1) {
		ERROR("failed asprintf(): %s", strerror(errno));
		return SR_ERR_SYS;
	}

	err = sr_install_module2(conn, path, mr->dir, mr->features, NULL,
				 "root", "wheel", 0660, NULL, NULL, 0);
	free(path);
	if (err == SR_ERR_EXISTS) {
		/* Probably loaded as a dependency */
		err = 0;

		/* Ensure all requested features are enabled */
		for (int i = 0; mr->features[i]; i++) {
			err = sr_enable_module_feature(conn, mr->name, mr->features[i]);
			if (err) {
				ERROR("failed enabling %s:%s, error %d", mr->name, mr->features[i], err);
			}
		}
	}

	return err;
}

sr_error_t srx_require_modules(sr_conn_ctx_t *conn, const struct srx_module_requirement *mrs)
{
	sr_error_t err = SR_ERR_OK;

	for (; mrs->name && !err; mrs++)
		err = srx_require_module(conn, mrs);

	return err;
}
