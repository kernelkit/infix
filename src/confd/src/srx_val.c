/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"

char *srx_get_str(sr_session_ctx_t *session, const char *fmt, ...)
{
	char *str = NULL;
	sr_val_t *val;
	char *xpath;
	va_list ap;
	int len;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap) + 1;
	va_end(ap);

	xpath = alloca(len);
	if (!xpath)
		return NULL;

	va_start(ap, fmt);
	vsnprintf(xpath, len, fmt, ap);
	va_end(ap);

	if (sr_get_item(session, xpath, 0, &val)) {
		ERROR("Failed reading string value from xpath %s", xpath);
		goto fail;
	}
	if (!val || val->type != SR_STRING_T)
		goto fail;

	str = strdup(val->data.string_val);
fail:
	if (val)
		sr_free_val(val);
	return str;
}
