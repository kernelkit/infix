/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"

int ietf_system_init(struct confd *confd)
{
	char *model[] = {
		YANG_PATH_"ietf-interfaces@2018-02-20.yang",
		YANG_PATH_"ietf-ip@2018-02-22.yang",
	};
	int rc, i;

	for (i = 0; i < NELEMS(model); i++) {
		if (rc = sr_install_module(confd->conn, model[i], NULL, NULL))
			goto err;
	}

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(rc));
	sr_unsubscribe(confd->sub);

	return rc;
}
