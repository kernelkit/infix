/* SPDX-License-Identifier: Apache-2.0 */

#ifndef MECH_COMMON_H_
#define MECH_COMMON_H_

#include <stdbool.h>
#include <cligen/cligen.h>
#include <clixon/clixon.h>
#include <clixon/clixon_backend.h>

/* debug.c */
void show_xml_node     (char *tag, cxobj *node);
void show_xml_node_vec (char *tag, cxobj **node_vec, size_t len);

void show_transaction  (char *tag, transaction_data td, bool show_data);

#endif /* MECH_COMMON_H_ */
