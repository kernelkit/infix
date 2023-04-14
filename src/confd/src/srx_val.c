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

static int get_vabool(sr_session_ctx_t *session, int *result, const char *fmt, va_list ap)
{
	sr_val_t *val = NULL;
	va_list apdup;
	int rc;

	va_copy(apdup, ap);
	rc = srx_vaget(session, fmt, apdup, &val, SR_BOOL_T);
	va_end(apdup);

	if (rc)
		return rc;

	*result = val->data.bool_val;
	sr_free_val(val);

	return 0;
}

int srx_get_bool(sr_session_ctx_t *session, int *result, const char *fmt, ...)
{
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = get_vabool(session, result, fmt, ap);
	va_end(ap);

	return rc;
}

int srx_enabled(sr_session_ctx_t *session, const char *fmt, ...)
{
	va_list ap;
	int v = 0;
	int rc;

	va_start(ap, fmt);
	rc = get_vabool(session, &v, fmt, ap);
	va_end(ap);

	return rc ? 0 : v;
}

int srx_get_int(sr_session_ctx_t *session, int *result, sr_val_type_t type, const char *fmt, ...)
{
	sr_val_t *val = NULL;
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = srx_vaget(session, fmt, ap, &val, type);
	va_end(ap);

	if (rc)
		return rc;
	rc = -1;

	switch (val->type) {
	case SR_INT8_T:
		*result = val->data.int8_val;
		break;
	case SR_UINT8_T:
		*result = val->data.uint8_val;
		break;
	case SR_INT16_T:
		*result = val->data.int16_val;
		break;
	case SR_UINT16_T:
		*result = val->data.uint16_val;
		break;
	case SR_INT32_T:
		*result = val->data.int32_val;
		break;
	case SR_UINT32_T:
		*result = val->data.uint32_val;
		break;
	case SR_INT64_T:
		*result = val->data.int64_val;
		break;
	case SR_UINT64_T:
		*result = val->data.uint64_val;
		break;
	default:
		goto fail;
	}

	rc = 0;
fail:
	sr_free_val(val);
	return rc;
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
