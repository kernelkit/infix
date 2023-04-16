/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_SRX_VAL_H_
#define CONFD_SRX_VAL_H_

#include "core.h"

#define SRX_GET_UINT8(s,v,fmt,...)  srx_get_int(s, &v, SR_UINT8_T, fmt, ##__VA_ARGS__)
#define SRX_GET_UINT32(s,v,fmt,...) srx_get_int(s, &v, SR_UINT32_T, fmt, ##__VA_ARGS__)

int srx_set_item(const struct ly_ctx *ctx, struct lyd_node **parent, int *first, char *xpath_base,
		 char *node, const char *fmt, ...);

char *srx_get_str  (sr_session_ctx_t *session, const char *fmt, ...);
int   srx_get_int  (sr_session_ctx_t *session, int *result, sr_val_type_t type, const char *fmt, ...);
int   srx_get_bool (sr_session_ctx_t *session, const char *fmt, ...);
int   srx_enabled  (sr_session_ctx_t *session, const char *fmt, ...);

#endif /* CONFD_SRX_VAL_H_ */
