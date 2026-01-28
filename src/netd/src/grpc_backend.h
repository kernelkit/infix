#ifndef NETD_GRPC_BACKEND_H_
#define NETD_GRPC_BACKEND_H_

#include "route.h"
#include "rip.h"

#ifdef __cplusplus
extern "C" {
#endif

int  grpc_backend_init(void);
void grpc_backend_fini(void);
int  grpc_backend_apply(struct route_head *routes, struct rip_config *rip);

#ifdef __cplusplus
}
#endif

#endif /* NETD_GRPC_BACKEND_H_ */
