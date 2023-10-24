/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"

static int factory_reset(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
	       const sr_val_t *input, const size_t input_cnt, sr_event_t event,
	       unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	DEBUG("%s", xpath);
	return systemf("factory -y");
}

int ietf_factory_default_init(struct confd *confd)
{
	int rc;

	REGISTER_RPC(confd->session, "/ietf-factory-default:factory-reset", factory_reset, NULL, &confd->fsub);
	return SR_ERR_OK;
fail:
	ERROR("failed: %s", sr_strerror(rc));
	return rc;
}
