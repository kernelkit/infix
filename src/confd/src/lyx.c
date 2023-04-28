/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdarg.h>
#include "core.h"

int lydx_new_path(const struct ly_ctx *ctx, struct lyd_node **parent, int *first,
		  char *xpath_base, char *node, const char *fmt, ...)
{
	char xpath[strlen(xpath_base) + strlen(node) + 2];
	va_list ap;
	size_t len;
	char *val;
	int rc;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap) + 1;
	va_end(ap);

	val = alloca(len);
	if (!val)
		return -1;

	snprintf(xpath, sizeof(xpath), "%s/%s", xpath_base, node);
	va_start(ap, fmt);
	vsnprintf(val, len, fmt, ap);
	va_end(ap);

	DEBUG("Setting first:%d xpath %s to %s", *first, xpath, val);

	if (*first)
		rc = lyd_new_path(NULL, ctx, xpath, val, 0, parent);
	else
		rc = lyd_new_path(*parent, NULL, xpath, val, 0, NULL);

	*first = 0;
	if (rc)
		ERROR("Failed building data tree, xpath %s, libyang error %d: %s",
		      xpath, rc, ly_errmsg(ctx));

	return rc;
}
