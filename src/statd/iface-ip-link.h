#ifndef STATD_IFACE_IP_LINK_H_
#define STATD_IFACE_IP_LINK_H_

#include <srx/lyx.h>

int ip_link_check_group(char *ifname, const char *group);
int ly_add_ip_link(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname);

#endif
