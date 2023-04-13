#ifndef CONFD_SR_EXT_H_
#define CONFD_SR_EXT_H_

#include <sysrepo.h>

struct srx_module_requirement {
	const char *dir;
	const char *name;
	const char *rev;
	const char **features;
};

sr_error_t srx_require_module(sr_conn_ctx_t *conn,
			      const struct srx_module_requirement *mr);
sr_error_t srx_require_modules(sr_conn_ctx_t *conn,
			       const struct srx_module_requirement *mrs);

#endif	/* CONFD_SR_EXT_H_ */
