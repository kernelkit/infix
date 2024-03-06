/* SPDX-License-Identifier: BSD-3-Clause */
#include <errno.h>
#include <stdarg.h>
#include <stdbool.h>
#include <sysrepo/values.h>

#include "common.h"

sr_error_t srx_get_diff(sr_session_ctx_t *session, struct lyd_node **treep)
{
	sr_error_t err = SR_ERR_OK;
	sr_change_iter_t *iter;
	struct lyd_node *tree;

	err = sr_dup_changes_iter(session, "/ietf-system:system//.", &iter);

	/* WARNING: sr_change_iter_t is opaque, we rely on the diff
	 * tree being the first member. */
	tree = *((struct lyd_node **)iter);

	if (lyd_dup_siblings(tree, NULL, LYD_DUP_RECURSIVE, treep))
		err = SR_ERR_LY;

	sr_free_change_iter(iter);
	return err;
}

int srx_set_item(sr_session_ctx_t *session, const sr_val_t *val, sr_edit_options_t opts,
		 const char *fmt, ...)
{
	char *xpath;
	va_list ap;
	size_t len;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap) + 1;
	va_end(ap);

	xpath = alloca(len);
	if (!xpath)
		return -1;

	va_start(ap, fmt);
	vsnprintf(xpath, len, fmt, ap);
	va_end(ap);

	return sr_set_item(session, xpath, val, opts);
}

int srx_set_str(sr_session_ctx_t *session, const char *str, sr_edit_options_t opts,
		const char *fmt, ...)
{
	char *xpath;
	va_list ap;
	size_t len;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap) + 1;
	va_end(ap);

	xpath = alloca(len);
	if (!xpath)
		return -1;

	va_start(ap, fmt);
	vsnprintf(xpath, len, fmt, ap);
	va_end(ap);

	return sr_set_item_str(session, xpath, str, NULL, opts);
}

static int srx_vaget(sr_session_ctx_t *session, const char *fmt, va_list ap, sr_val_type_t type,
		     sr_val_t **val, size_t *cnt, bool logerr)
{
	va_list apdup;
	char *xpath;
	int len;
	int rc;

	va_copy(apdup, ap);
	len = vsnprintf(NULL, 0, fmt, apdup) + 1;
	va_end(apdup);

	xpath = alloca(len);
	if (!xpath)
		return -1;

	va_copy(apdup, ap);
	vsnprintf(xpath, len, fmt, apdup);
	va_end(apdup);

	rc = sr_get_items(session, xpath, 0, 0, val, cnt);
	if (rc) {
		if (logerr)
			ERROR("Failed reading xpath %s: %s", xpath, sr_strerror(rc));
		return rc;
	}

	if (*cnt == 0) {
		errno = ENODATA;
		return -1;
	} else if (*cnt > 1) {
		sr_free_values(*val, *cnt);
		errno = EOVERFLOW;
		return -1;
	}

	if (type != SR_UNKNOWN_T && val[0]->type != type) {
		sr_free_values(*val, *cnt);
		errno = EINVAL;
		return -1;
	}

	return 0;
}

static int get_vabool(sr_session_ctx_t *session, int *value, const char *fmt, va_list ap, bool logerr)
{
	sr_val_t *val = NULL;
	size_t cnt = 0;
	va_list apdup;
	int rc;

	va_copy(apdup, ap);
	rc = srx_vaget(session, fmt, apdup, SR_BOOL_T, &val, &cnt, logerr);
	va_end(apdup);

	if (rc)
		return rc;

	*value = val->data.bool_val;
	sr_free_values(val, cnt);

	return 0;
}

int srx_get_bool(sr_session_ctx_t *session, int *value, const char *fmt, ...)
{
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = get_vabool(session, value, fmt, ap, false);
	va_end(ap);

	return rc;
}

int srx_enabled(sr_session_ctx_t *session, const char *fmt, ...)
{
	va_list ap;
	int v = 0;
	int rc;

	va_start(ap, fmt);
	rc = get_vabool(session, &v, fmt, ap, true);
	va_end(ap);

	return rc ? 0 : v;
}

int srx_get_int(sr_session_ctx_t *session, int *value, sr_val_type_t type, const char *fmt, ...)
{
	sr_val_t *val = NULL;
	size_t cnt = 0;
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = srx_vaget(session, fmt, ap, type, &val, &cnt, false);
	va_end(ap);

	if (rc)
		return rc;
	rc = -1;

	switch (val->type) {
	case SR_INT8_T:
		*value = val->data.int8_val;
		break;
	case SR_UINT8_T:
		*value = val->data.uint8_val;
		break;
	case SR_INT16_T:
		*value = val->data.int16_val;
		break;
	case SR_UINT16_T:
		*value = val->data.uint16_val;
		break;
	case SR_INT32_T:
		*value = val->data.int32_val;
		break;
	case SR_UINT32_T:
		*value = val->data.uint32_val;
		break;
	case SR_INT64_T:
		*value = val->data.int64_val;
		break;
	case SR_UINT64_T:
		*value = val->data.uint64_val;
		break;
	default:
		goto fail;
	}

	rc = 0;
fail:
	sr_free_values(val, cnt);
	return rc;
}

char *srx_get_str(sr_session_ctx_t *session, const char *fmt, ...)
{
	sr_val_t *val = NULL;
	char *str = NULL;
	size_t cnt = 0;
	va_list ap;

	va_start(ap, fmt);
	if (srx_vaget(session, fmt, ap, SR_UNKNOWN_T, &val, &cnt, false))
		goto fail;

	str = sr_val_to_str(val);
	sr_free_values(val, cnt);
fail:
	va_end(ap);
	return str;
}

int srx_vnitems(sr_session_ctx_t *session, size_t *cntp, const char *fmt, va_list ap)
{
	sr_val_t *val = NULL;
	va_list apdup;
	int rc;

	errno = 0;

	va_copy(apdup, ap);
	rc = srx_vaget(session, fmt, apdup, SR_UNKNOWN_T, &val, cntp, false);
	va_end(apdup);

	if (rc)
		/* We don't care about srx_vaget's extra validation,
		 * we're only interested in the number of items
		 * found.
		 */
		return errno ? 0 : rc;

	sr_free_values(val, *cntp);
	return 0;
}

int srx_nitems(sr_session_ctx_t *session, size_t *cntp, const char *fmt, ...)
{
	va_list ap;
	int rc;

	va_start(ap, fmt);
	rc = srx_vnitems(session, cntp, fmt, ap);
	va_end(ap);

	return rc;
}
