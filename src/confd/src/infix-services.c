/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <pwd.h>
#include <stdarg.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include <srx/common.h>
#include <srx/lyx.h>
#include <srx/srx_module.h>
#include <srx/srx_val.h>

#include "core.h"


static const struct srx_module_requirement reqs[] = {
	{ .dir = YANG_PATH_, .name = "infix-services",      .rev = "2024-04-08" },
	{ .dir = YANG_PATH_, .name = "ieee802-dot1ab-lldp", .rev = "2022-03-15" },
	{ .dir = YANG_PATH_, .name = "infix-lldp",          .rev = "2023-08-23" },
	{ NULL }
};

static int svc_change(sr_session_ctx_t *session, sr_event_t event, const char *xpath,
		      const char *name, const char *sub, ...)
{
	char path[strlen(xpath) + 10];
	struct lyd_node *diff, *srv;
	sr_error_t err = 0;
	const char *svc;
	sr_data_t *cfg;
	va_list ap;
	int ena;

	switch (event) {
	case SR_EV_DONE:
		break;
	case SR_EV_CHANGE:
	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	snprintf(path, sizeof(path), "%s//.", xpath);
	err = sr_get_data(session, path, 0, 0, 0, &cfg);
	if (err) {
		ERROR("no data for %s", path);
		goto err_abandon;
	}

	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;

	srv = lydx_get_descendant(cfg->tree, name, sub, NULL);
	if (!srv) {
		ERROR("Cannot find %s subtree", name);
		return -1;
	}

	ena = lydx_is_enabled(srv, "enabled");
	va_start(ap, sub);
	while ((svc = va_arg(ap, const char *))) {
		if (systemf("initctl -nbq %s %s", ena ? "enable" : "disable", svc))
			ERROR("Failed %s %s", ena ? "enabling" : "disabling", name);
	}
	va_end(ap);

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:

	return err;
}

static int mdns_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	return svc_change(session, event, xpath, "mdns", NULL, "avahi", NULL);
}

static int ttyd_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	return svc_change(session, event, xpath, "web", "console", "ttyd", NULL);
}

static int lldp_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	return svc_change(session, event, xpath, "lldp", NULL, "lldpd", NULL);
}

static int web_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	return svc_change(session, event, xpath, "web", NULL, "nginx", "ttyd", "netbrowse", NULL);
}

int infix_services_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, reqs);
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:mdns",
			0, mdns_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:web",
			0, web_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:web/infix-services:console",
			0, ttyd_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ieee802-dot1ab-lldp", "/ieee802-dot1ab-lldp:lldp",
			0, lldp_change, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
