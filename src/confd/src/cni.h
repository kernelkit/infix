/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_CNI_H_
#define CONFD_CNI_H_
#include "dagger.h"

#ifdef CONTAINERS

pid_t cni_find(const char *ifname);
FILE *cni_popen(const char *fmt, const char *ifname);

int cni_netdag_gen_iface(struct dagger *net, const char *ifname, struct lyd_node *dif, struct lyd_node *cif);
int cni_ifchange_cand_infer_type(sr_session_ctx_t *session, const char *path);

#else
static inline pid_t cni_find(const char *ifname) { return 0; }
static inline FILE *cni_popen(const char *fmt, const char *ifname) { return popenf("re", fmt, ifname); }

static inline int cni_netdag_gen_iface(struct dagger *net, const char *ifname, struct lyd_node *dif, struct lyd_node *cif) { return 0; }
static inline int cni_ifchange_cand_infer_type(sr_session_ctx_t *session, const char *path) { return 0; }
#endif

#endif /* CONFD_CNI_H_ */
