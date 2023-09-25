#ifndef STATD_IFACE_IP_ADDR_H_
#define STATD_IFACE_IP_ADDR_H_

#include <srx/lyx.h>

int ly_add_ip_addr(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname);

#endif
