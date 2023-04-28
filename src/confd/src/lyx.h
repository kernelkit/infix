/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_LY_EXT_H_
#define CONFD_LY_EXT_H_

#include <libyang/libyang.h>

int lydx_new_path(const struct ly_ctx *ctx, struct lyd_node **parent, int *first, char *xpath_base,
		  char *node, const char *fmt, ...)
	__attribute__ ((format (printf, 6, 7)));

#endif	/* CONFD_LY_EXT_H_ */
