/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"
#include "sr_ext.h"

const struct srx_module_requirement ietf_if_reqs[] = {
	{ .dir = YANG_PATH_, .name = "ietf-interfaces", .rev = "2018-02-20" },
	{ .dir = YANG_PATH_, .name = "iana-if-type", .rev = "2023-01-26" },
	{ .dir = YANG_PATH_, .name = "ietf-ip", .rev = "2018-02-22" },

	{ NULL }
};

int ietf_interfaces_init(struct confd *confd)
{
	int err;

	err = srx_require_modules(confd->conn, ietf_if_reqs);
	if (err)
		goto err;

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(err));
	sr_unsubscribe(confd->sub);

	return err;
}
