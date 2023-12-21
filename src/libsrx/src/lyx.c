/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdarg.h>
#include "common.h"
#include "lyx.h"

enum lydx_op lydx_get_op(struct lyd_node *node)
{
	const char *opstr = NULL;

	for (; !opstr && node; node = lyd_parent(node))
		opstr = lydx_get_mattr(node, "yang:operation");

	if (opstr && !strcmp(opstr, "create"))
		return LYDX_OP_CREATE;

	if (opstr && !strcmp(opstr, "delete"))
		return LYDX_OP_DELETE;

	if (opstr && !strcmp(opstr, "replace"))
		return LYDX_OP_REPLACE;

	return LYDX_OP_NONE;
}

void lydx_diff_print(struct lydx_diff *nd, FILE *fp)
{
	static const char opchar[] = {
		[LYDX_OP_CREATE]  = '+',
		[LYDX_OP_DELETE]  = '-',
		[LYDX_OP_REPLACE] = '|',
		[LYDX_OP_NONE]    = '%',
	};

	fprintf(fp, "%c%s %s%s ->%s%s\n",
		opchar[nd->op], nd->modified ? "(mod)" : "",
		nd->old ? : "", nd->was_default ? "(D)" : "",
		nd->new ? : "", nd->is_default ? "(D)" : "");
}

bool lydx_get_diff(struct lyd_node *node, struct lydx_diff *nd)
{

	const char *old, *odflt;

	memset(nd, 0, sizeof(*nd));

	if (!node)
		goto out;

	nd->op = lydx_get_op(node);
	nd->val = lyd_get_value(node);

	switch (nd->op) {
	case LYDX_OP_DELETE:
		nd->old = nd->val;
		nd->was_default = !!(node->flags & LYD_DEFAULT);
		break;

	default:
		nd->new = nd->val;
		nd->is_default = !!(node->flags & LYD_DEFAULT);

		if (nd->op == LYDX_OP_CREATE)
			break;

		old = lydx_get_mattr(node, "yang:orig-value");
		nd->old = old ? : nd->new;
		nd->modified = old ? true : false;

	        odflt = lydx_get_mattr(node, "yang:orig-default");
		nd->was_default = odflt && !strcmp(odflt, "true");
	}

	nd->modified = \
		(nd->old && !nd->was_default) ||
		(nd->new && !nd->is_default);

out:
	return nd->modified;
}

struct lyd_node *lydx_get_sibling(struct lyd_node *sibling, const char *name)
{
	struct lyd_node *node;

	if (!sibling)
		return NULL;

	LY_LIST_FOR(sibling, node) {
		if (node->schema && node->schema->name &&
		    !strcmp(node->schema->name, name))
			return node;
	}

	return NULL;
}

struct lyd_node *lydx_get_child(struct lyd_node *parent, const char *name)
{
	return lydx_get_sibling(lyd_child(parent), name);
}

struct lyd_node *lydx_get_descendant(struct lyd_node *from, ...)
{
	struct lyd_node *node = NULL;
	const char *name;
	va_list ap;

	va_start(ap, from);
	while ((name = va_arg(ap, const char *))) {
		node = lydx_get_sibling(from, name);
		if (!node)
			break;

		from = lyd_child(node);
	}
	va_end(ap);

	return node;
}

const char *lydx_get_mattr(struct lyd_node *node, const char *name)
{
	struct lyd_meta *meta;

	if (!node)
		return NULL;

	meta = lyd_find_meta(node->meta, NULL, name);
	if (!meta)
		return NULL;

	return lyd_get_meta_value(meta);
}

const char *lydx_get_attr(struct lyd_node *sibling, const char *name)
{
	struct lyd_node *node;

	node = lydx_get_sibling(sibling, name);
	if (!node)
		return NULL;

	return lyd_get_value(node);
}

const char *lydx_get_cattr(struct lyd_node *parent, const char *name)
{
	return lydx_get_attr(lyd_child(parent), name);
}

const char *lydx_get_vattrf(struct lyd_node *sibling, const char *namefmt, va_list ap)
{
	const char *val;
	char *name;

	if (vasprintf(&name, namefmt, ap) < 0)
		return NULL;

	val = lydx_get_attr(sibling, name);
	free(name);

	return val;
}

const char *lydx_get_attrf(struct lyd_node *sibling, const char *namefmt, ...)
{
	const char *val;
	va_list ap;

	va_start(ap, namefmt);
	val = lydx_get_vattrf(sibling, namefmt, ap);
	va_end(ap);

	return val;
}

bool lydx_get_bool(struct lyd_node *parent, const char *name)
{
	const char *value = lydx_get_cattr(parent,name);
	if(!value)
		return false;
	if(!strcmp(value, "true"))
		return true;
	return false;
}

int lydx_new_path(const struct ly_ctx *ctx, struct lyd_node **parent,
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

	DEBUG("Setting xpath %s to %s", xpath, val);

	rc = lyd_new_path(*parent, NULL, xpath, val, 0, NULL);
	if (rc)
		ERROR("Failed building data tree, xpath %s, libyang error %d: %s",
		      xpath, rc, ly_errmsg(ctx));

	return rc;
}
