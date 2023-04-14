/* SPDX-License-Identifier: BSD-3-Clause */

#include <libyang/libyang.h>

#include "sr_ext.h"

sr_error_t srx_require_module(sr_conn_ctx_t *conn,
			      const struct srx_module_requirement *mr)
{
	sr_error_t err = SR_ERR_OK;
	const struct ly_ctx *ly;
	struct lys_module *mod;
	const char **f;
	char *path;

	ly = sr_acquire_context(conn);
	if (!ly)
		return SR_ERR_INVAL_ARG;

	mod = ly_ctx_get_module(ly, mr->name, mr->rev);
	sr_release_context(conn);

	if (mod && mod->implemented) {
		if (!mr->features)
			return SR_ERR_OK;

		for (f = mr->features; *f; f++) {
			switch (lys_feature_value(mod, *f)) {
			case LY_SUCCESS:
				continue;
			case LY_ENOT:
				err = sr_enable_module_feature(conn, mr->name, *f);
				if (err)
					return err;
				break;
			default:
				return SR_ERR_UNSUPPORTED;
			}
		}
	} else {
		/* `search_dirs` argument is ignored by sysrepo 2.2.60,
		 * so we supply the full path instead.
		 */
		asprintf(&path, "%s/%s%s%s.yang", mr->dir ? : "", mr->name,
			 mr->rev ? "@" : "", mr->rev ? : "");
		err = sr_install_module(conn, path, NULL, mr->features);
		free(path);
	}

	return err;
}

sr_error_t srx_require_modules(sr_conn_ctx_t *conn,
			       const struct srx_module_requirement *mrs)
{
	sr_error_t err = SR_ERR_OK;

	for (; mrs->name && !err; mrs++)
		err = srx_require_module(conn, mrs);

	return err;
}
