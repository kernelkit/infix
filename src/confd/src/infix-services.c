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

static sr_data_t *get(sr_session_ctx_t *session, sr_event_t event, const char *xpath,
		      struct lyd_node **srv, ...)
{
	char path[strlen(xpath) + 4];
	sr_data_t *cfg = NULL;
	va_list ap;

	if (event != SR_EV_DONE)
		return NULL;  /* Don't care about CHANGE, ABORT, etc. */

	snprintf(path, sizeof(path), "%s//.", xpath);
	if (sr_get_data(session, path, 0, 0, 0, &cfg)) {
		ERROR("no data for %s", path);
		return NULL;
	}

	va_start(ap, srv);
	*srv = lydx_vdescend(cfg->tree, ap);
	va_end(ap);

	if (!*srv) {
		sr_release_data(cfg);
		return NULL;
	}

	return cfg;
}

static int put(sr_data_t *cfg, struct lyd_node *srv)
{
	sr_release_data(cfg);
	return SR_ERR_OK;
}

static int svc_change(sr_session_ctx_t *session, sr_event_t event, const char *xpath,
		      const char *name, const char *svc)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	int ena;

	cfg = get(session, event, xpath, &srv, name, NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled");
	if (systemf("initctl -nbq %s %s", ena ? "enable" : "disable", svc))
		ERROR("Failed %s %s", ena ? "enabling" : "disabling", name);

	return put(cfg, srv);
}

static void svc_enadis(int ena, const char *svc)
{
	int isweb = fexistf("/etc/nginx/available/%s.conf", svc);

	if (ena) {
		if (isweb)
			systemf("ln -sf ../available/%s.conf /etc/nginx/enabled/", svc);
		systemf("initctl -nbq enable %s", svc);
	} else {
		if (isweb)
			systemf("rm -f /etc/nginx/enabled/%s.conf", svc);
		systemf("initctl -nbq disable %s", svc);
	}
	systemf("initctl -nbq touch avahi");
	systemf("initctl -nbq touch nginx");
}

static int mdns_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	int ena;

	cfg = get(session, event, xpath, &srv, "mdns", NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled");
	svc_enadis(ena, "mdns-alias");
	svc_enadis(ena, "avahi");

	return put(cfg, srv);
}

static int lldp_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	return svc_change(session, event, xpath, "lldp", "lldpd");
}

static int ttyd_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;

	cfg = get(session, event, xpath, &srv, "web", "console", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), "ttyd");

	return put(cfg, srv);
}

static int netbrowse_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;

	cfg = get(session, event, xpath, &srv, "web", "netbrowse", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), "netbrowse");

	return put(cfg, srv);
}

static int web_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		      const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	int ena;

	cfg = get(session, event, xpath, &srv, "web", NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled");
	if (ena) {
		svc_enadis(srx_enabled(session, "%s/console/enabled", xpath), "ttyd");
		svc_enadis(srx_enabled(session, "%s/netbrowse/enabled", xpath), "netbrowse");
	} else {
		svc_enadis(0, "ttyd");
		svc_enadis(0, "netbrowse");
	}
	svc_enadis(ena, "nginx"); /* fake it, not nginx .conf */

	return put(cfg, srv);
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
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:web/infix-services:netbrowse",
			0, netbrowse_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ieee802-dot1ab-lldp", "/ieee802-dot1ab-lldp:lldp",
			0, lldp_change, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
