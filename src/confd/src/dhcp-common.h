/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef DHCP_COMMON_H_
#define DHCP_COMMON_H_

#include <libyang/libyang.h>

/* DHCP option lookup and composition helpers */
int dhcp_option_lookup(const struct lyd_node *id);
char *dhcp_hostname(struct lyd_node *cfg, char *str, size_t len);
char *dhcp_fqdn(const char *val, char *str, size_t len);
char *dhcp_compose_option(struct lyd_node *cfg, const char *ifname, struct lyd_node *id,
			  const char *val, const char *hex, char *option, size_t len,
			  char *(*ip_cache_cb)(const char *, char *, size_t));
char *dhcp_compose_options(struct lyd_node *cfg, const char *ifname, char **options,
			   struct lyd_node *id, const char *val, const char *hex,
			   char *(*ip_cache_cb)(const char *, char *, size_t));

#endif /* DHCP_COMMON_H_ */
