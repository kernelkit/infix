/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef NETD_H_
#define NETD_H_

#include <arpa/inet.h>
#include <net/if.h>
#include <netinet/in.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <syslog.h>
#include <unistd.h>
#include <libite/queue.h>

extern int debug;

#define LOG(level, fmt, args...) syslog(level, fmt, ##args)
#define ERROR(fmt, args...)      LOG(LOG_ERR, fmt, ##args)
#define INFO(fmt, args...)       LOG(LOG_INFO, fmt, ##args)
#define DEBUG(fmt, args...)      do { if (debug) LOG(LOG_DEBUG, fmt, ##args); } while (0)

#define CONF_DIR    "/etc/netd/conf.d"

#endif /* NETD_H_ */
