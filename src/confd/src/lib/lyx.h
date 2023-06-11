/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_LY_EXT_H_
#define CONFD_LY_EXT_H_

#include <stdbool.h>
#include <stdio.h>

#include <libyang/libyang.h>

#define LYX_LIST_FOR_EACH(_from, _iter, _name) \
	LY_LIST_FOR(_from, _iter) if (!strcmp((_iter)->schema->name, _name))

enum lydx_op {
	LYDX_OP_NONE,
	LYDX_OP_CREATE,
	LYDX_OP_DELETE,
	LYDX_OP_REPLACE,
};

struct lydx_diff {
	enum lydx_op op;
	bool modified;
	bool is_default;
	bool was_default;

	const char *old;
	const char *val;
	const char *new;
};
void lydx_diff_print(struct lydx_diff *nd, FILE *fp);

enum lydx_op lydx_get_op(struct lyd_node *node);
bool lydx_get_diff(struct lyd_node *node, struct lydx_diff *nd);

struct lyd_node *lydx_get_sibling(struct lyd_node *sibling, const char *name);
struct lyd_node *lydx_get_child(struct lyd_node *parent, const char *name);
struct lyd_node *lydx_get_descendant(struct lyd_node *from, ...);

const char *lydx_get_mattr(struct lyd_node *node, const char *name);
const char *lydx_get_attr(struct lyd_node *sibling, const char *name);
const char *lydx_get_cattr(struct lyd_node *parent, const char *name);

const char *lydx_get_vattrf(struct lyd_node *sibling, const char *namefmt, va_list ap);
const char *lydx_get_attrf(struct lyd_node *sibling, const char *namefmt, ...)
	__attribute__ ((format (printf, 2, 3)));

int lydx_new_path(const struct ly_ctx *ctx, struct lyd_node **parent, int *first, char *xpath_base,
		  char *node, const char *fmt, ...)
	__attribute__ ((format (printf, 6, 7)));

static inline int lydx_is_enabled(struct lyd_node *parent, const char *name)
{
	const char *attr;

	attr = lydx_get_cattr(parent, name);
	if (!attr || strcmp(attr, "true"))
		return 0;

	return 1;
}

#endif	/* CONFD_LY_EXT_H_ */
