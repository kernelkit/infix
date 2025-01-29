/* SPDX-License-Identifier: BSD-3-Clause */

#include <libyang/libyang.h>

int dhcp_option_lookup(const struct lyd_node *id)
{
	const struct lysc_type_enum *enum_type;
	const struct lysc_type_union *uni;
	const struct lysc_node_leaf *leaf;
	const struct lysc_node *schema;
	const struct lysc_type *type;
	LY_ARRAY_COUNT_TYPE u, e;
	const char *name;

	schema = id->schema;
	if (!schema || schema->nodetype != LYS_LEAF)
		return -1;

	leaf = (const struct lysc_node_leaf *)schema;
	type = leaf->type;

	if (type->basetype != LY_TYPE_UNION)
		return -1;	/* We expect a union type */

	uni = (const struct lysc_type_union *)type;
	name = lyd_get_value(id);

	/* Look through each type in the union */
	for (u = 0; u < LY_ARRAY_COUNT(uni->types); u++) {
		type = uni->types[u];

		if (type->basetype == LY_TYPE_ENUM) {
			enum_type = (const struct lysc_type_enum *)type;

			for (e = 0; e < LY_ARRAY_COUNT(enum_type->enums); e++) {
				if (!strcmp(enum_type->enums[e].name, name))
					return enum_type->enums[e].value;
			}
		} else if (type->basetype == LY_TYPE_UINT8) {
			char *endptr;
			long val;

			val = strtol(name, &endptr, 10);
			if (*endptr == 0 && val > 0 && val < 255)
				return (int)val;
		}
	}

	return -1;
}
