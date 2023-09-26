#ifndef STATD_IFACE_ETHTOOL_H_
#define STATD_IFACE_ETHTOOL_H_

#include <srx/lyx.h>

int ly_add_ethtool(const struct ly_ctx *ctx, struct lyd_node **parent, char *ifname);

#endif
