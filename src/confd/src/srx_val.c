/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdarg.h>
#include "core.h"

static int srx_vaget(sr_session_ctx_t *session, const char *fmt, va_list ap, sr_val_t **val, sr_val_type_t type)
{
	va_list apdup;
	char *xpath;
	int len;

	va_copy(apdup, ap);
	len = vsnprintf(NULL, 0, fmt, apdup) + 1;
	va_end(apdup);

	xpath = alloca(len);
	if (!xpath)
		return -1;

	va_copy(apdup, ap);
	vsnprintf(xpath, len, fmt, apdup);
	va_end(apdup);

	if (sr_get_item(session, xpath, 0, val)) {
		ERROR("Failed reading xpath %s", xpath);
		return -1;
	}

	if (type != SR_UNKNOWN_T && (*val)->type != type)
		return -1;

	return 0;
}

int srx_get_bool(sr_session_ctx_t *session, const char *fmt, ...)
{
	sr_val_t *val = NULL;
	int result = -1;
	va_list ap;

	va_start(ap, fmt);
	if (srx_vaget(session, fmt, ap, &val, SR_BOOL_T))
		goto fail;

	result = val->data.bool_val;
	sr_free_val(val);
fail:
	va_end(ap);
	return result;
}


char *srx_get_str(sr_session_ctx_t *session, const char *fmt, ...)
{
	sr_val_t *val = NULL;
	char *str = NULL;
	va_list ap;

	va_start(ap, fmt);
	if (srx_vaget(session, fmt, ap, &val, SR_STRING_T))
		goto fail;

	str = sr_val_to_str(val);
	sr_free_val(val);
fail:
	va_end(ap);
	return str;
}
