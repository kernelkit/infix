/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <pwd.h>
#include <sched.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"

static int rpc(sr_session_ctx_t *session, uint32_t sub_id, const char *xpath,
	       const sr_val_t *input, const size_t input_cnt, sr_event_t event,
	       unsigned request_id, sr_val_t **output, size_t *output_cnt, void *priv)
{
	int rc;

	DEBUG("%s", xpath);

	sr_session_switch_ds(session, SR_DS_RUNNING);
	rc = sr_copy_config(session, NULL, SR_DS_FACTORY_DEFAULT, 60000);
	if (rc) {
		sr_session_set_netconf_error(session, "application", "operation-failed",
                         NULL, xpath, sr_strerror(rc), 0);
		rc = SR_ERR_OPERATION_FAILED;
	}

	return rc;
}

int infix_factory_init(struct confd *confd)
{
	int rc;
	REGISTER_RPC(confd->session, "/infix-factory-default:factory-default", rpc, NULL, &confd->fsub);
	return SR_ERR_OK;
fail:
	ERROR("failed: %s", sr_strerror(rc));
	return rc;
}
