/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <pwd.h>
#include <stdarg.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include <srx/common.h>
#include <srx/helpers.h>
#include <srx/lyx.h>
#include <srx/srx_val.h>

#include "core.h"
#include <sysrepo_types.h>

#define GENERATE_ENUM(ENUM)      ENUM,
#define GENERATE_STRING(STRING) #STRING,

#define LLDP_CONFIG "/etc/lldpd.d/confd.conf"
#define LLDP_CONFIG_NEXT LLDP_CONFIG"+"

#define FOREACH_SVC(SVC)			\
        SVC(none)				\
        SVC(ssh)				\
        SVC(netconf)				\
        SVC(restconf)				\
        SVC(web)				\
        SVC(ttyd)				\
        SVC(netbrowse)				\
        SVC(all)	/* must be last entry */

#define SSH_BASE            "/etc/ssh"
#define SSHD_CONFIG_BASE    SSH_BASE "/sshd_config.d"
#define SSHD_CONFIG_LISTEN  SSHD_CONFIG_BASE "/listen.conf"
#define SSHD_CONFIG_HOSTKEY SSHD_CONFIG_BASE "/host-keys.conf"
#define LLDP_XPATH          "/ieee802-dot1ab-lldp:lldp"
#define SSH_XPATH           "/infix-services:ssh"
#define MDNS_XPATH          "/infix-services:mdns"
#define WEB_XPATH           "/infix-services:web"
#define WEB_RESTCONF_XPATH  WEB_XPATH"/restconf"
#define WEB_NETBROWSE_XPATH WEB_XPATH"/netbrowse"
#define WEB_CONSOLE_XPATH   WEB_XPATH"/console"
typedef enum {
    FOREACH_SVC(GENERATE_ENUM)
} svc;

static const char *name[] = {
    FOREACH_SVC(GENERATE_STRING)
};

struct mdns_svc {
	svc   svc;
	char *name;
	char *type;
	int   port;
	char *desc;
	char *text;
} services[] = {
	{ web,      "https",    "_https._tcp",        443, "Web Management Interface", "adminurl=https://%s.local" },
	{ ttyd,     "ttyd",     "_https._tcp",        443, "Web Console Interface",    "adminurl=https://%s.local:7681" },
	{ web,      "http",     "_http._tcp",          80, "Web Management Interface", "adminurl=http://%s.local" },
	{ netconf,  "netconf",  "_netconf-ssh._tcp",  830, "NETCONF (XML/SSH)", NULL },
	{ restconf, "restconf", "_restconf-tls._tcp", 443, "RESTCONF (JSON/HTTP)",  NULL },
	{ ssh,      "sftp-ssh", "_sftp-ssh._tcp",      22, "Secure file transfer (FTP/SSH)", NULL },
	{ ssh,      "ssh",      "_ssh._tcp",           22, "Secure shell command line interface (CLI)", NULL },
};

/*
 * On hostname changes we need to update the mDNS records, in particular
 * the ones advertising an adminurl (standarized by Apple), because they
 * include the fqdn in the URL.
 *
 * XXX: when the web managment interface is in place we can change the
 *      adminurl to include 'admin@%s.local' to pre-populate the default
 *      username in the login dialog.
 */
static int mdns_records(const char *cmd, svc type)
{
	char hostname[MAXHOSTNAMELEN + 1];

	if (gethostname(hostname, sizeof(hostname))) {
		ERRNO("failed getting system hostname");
		return SR_ERR_SYS;
	}

	for (size_t i = 0; i < NELEMS(services); i++) {
		struct mdns_svc *srv = &services[i];
		char buf[256] = "";

		if (type != all && srv->svc != type)
			continue;

		if (srv->text)
			snprintf(buf, sizeof(buf), srv->text, hostname);

		systemf("/usr/libexec/confd/gen-service %s %s %s %s %d \"%s\" %s", cmd,
			hostname, srv->name, srv->type, srv->port, srv->desc, buf);
	}

	return SR_ERR_OK;
}

static sr_data_t *get(sr_session_ctx_t *session, sr_event_t event, const char *xpath,
		      struct lyd_node **srv, ...)
{
	char path[strlen(xpath) + 4];
	sr_data_t *cfg = NULL;
	va_list ap;

	if (event != SR_EV_DONE)
		return NULL;  /* Don't care about CHANGE, ABORT, etc. */

	snprintf(path, sizeof(path), "%s//.", xpath);
	if (sr_get_data(session, path, 0, 0, 0, &cfg) || !cfg) {
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

static int put(sr_data_t *cfg)
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
	if (ena)
		systemf("initctl -nbq touch %s", svc); /* in case already enabled */

	return put(cfg);
}

static void svc_enadis(int ena, svc type, const char *svc)
{
	int isweb, isapp;

	if (!svc)
		svc = name[type];
	isweb = fexistf("/etc/nginx/available/%s.conf", svc);
	isapp = fexistf("/etc/nginx/%s.app", svc);

	if (ena) {
		if (isweb)
			systemf("ln -sf ../available/%s.conf /etc/nginx/enabled/", svc);
		if (isapp)
			systemf("ln -sf ../%s.app /etc/nginx/app/%s.conf", svc, svc);
		systemf("initctl -nbq enable %s", svc);
		systemf("initctl -nbq touch %s", svc); /* in case already enabled */
	} else {
		if (isweb)
			systemf("rm -f /etc/nginx/enabled/%s.conf", svc);
		if (isapp)
			systemf("rm -f /etc/nginx/app/%s.conf", svc);
		systemf("initctl -nbq disable %s", svc);
	}

	if (type != none)
		mdns_records(ena ? "add" : "delete", type);

	systemf("initctl -nbq touch avahi");
	systemf("initctl -nbq touch nginx");
}
/* TODO Handle in dependency tracking
static int hostname_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	return mdns_records("update", all);
}
*/
static void fput_list(FILE *fp, struct lyd_node *cfg, const char *list, const char *heading)
{
	const char *prefix = heading;
	struct lyd_node *node;

	LYX_LIST_FOR_EACH(lyd_child(cfg), node, list) {
		fprintf(fp, "%s%s", prefix, lyd_get_value(node));
		prefix = ",";
	}

	if (prefix != heading)
		fprintf(fp, "\n");
}

#define AVAHI_CONF "/etc/avahi/avahi-daemon.conf"

static void mdns_conf(struct lyd_node *cfg)
{
	struct lyd_node *ctx;
	FILE *fp;

	fp = fopen(AVAHI_CONF, "w");
	if (!fp) {
		ERRNO("failed creating %s", AVAHI_CONF);
		return;
	}

	fprintf(fp, "# Generated by Infix confd\n"
		"[server]\n"
		"domain-name=%s\n"
		"use-ipv4=yes\n"
		"use-ipv6=yes\n", lydx_get_cattr(cfg, "domain"));

	ctx = lydx_get_descendant(lyd_child(cfg), "interfaces", NULL);
	if (ctx) {
		fput_list(fp, ctx, "allow", "allow-interfaces=");
		fput_list(fp, ctx, "deny", "deny-interfaces=");
	}

	fprintf(fp,
		"ratelimit-interval-usec=1000000\n"
		"ratelimit-burst=1000\n");

	fprintf(fp, "\n[wide-area]\n");
	/* nop */
	fprintf(fp, "\n[publish]\n");
	/* nop */
	fprintf(fp, "\n[reflector]\n");
	ctx = lydx_get_descendant(lyd_child(cfg), "reflector", NULL);
	if (ctx) {
		fprintf(fp, "enable-reflector=%s\n", lydx_is_enabled(ctx, "enabled") ? "on" : "off");
		fput_list(fp, ctx, "service-filter", "reflect-filters=");
	}

	fprintf(fp, "\n[rlimits]\n");
	/* nop */

	fclose(fp);
}

static void mdns_cname(sr_session_ctx_t *session)
{
	int ena = srx_enabled(session, "/infix-services:mdns/enabled");

	if (ena) {
		int www = srx_enabled(session, "/infix-services:web/netbrowse/enabled");
		const char *hostname = fgetkey("/etc/os-release", "DEFAULT_HOSTNAME");

		if (hostname || www) {
			FILE *fp;

			fp = fopen("/etc/default/mdns-alias", "w");
			if (fp) {
				fprintf(fp, "MDNS_ALIAS_ARGS=\"%s%s %s\"\n",
					hostname ?: "", hostname ? ".local" : "",
					www ? "network.local" : "");
				fclose(fp);
			} else {
				ERRNO("failed updating mDNS aliases");
				ena = 0;
			}
		} else
			ena = 0; /* nothing to advertise */
	}

	svc_enadis(ena, none, "mdns-alias");
}

static int mdns_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	int ena;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, MDNS_XPATH))
		return SR_ERR_OK;

	cfg = get(session, event, MDNS_XPATH, &srv, "mdns", NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled");
	if (ena) {
		/* Generate/update avahi-daemon.conf */
		mdns_conf(srv);

		/* Generate/update basic mDNS service records */
		mdns_records("update", all);
	}

	svc_enadis(ena, none, "avahi");
	mdns_cname(session);

	return put(cfg);
}

static int lldp_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *node = NULL;
	struct lyd_node *subnode;

	if (diff && !lydx_get_xpathf(diff, LLDP_XPATH))
		return SR_ERR_OK;

	switch (event) {
	case SR_EV_ENABLED:
	case SR_EV_CHANGE:
		node = lydx_get_xpathf(config, LLDP_XPATH);
		if (lydx_is_enabled(node, "enabled")){
			const char *tx_interval = lydx_get_cattr(node, "message-tx-interval");
			FILE *fp = fopen(LLDP_CONFIG_NEXT, "w");
			if (!fp) {
				ERRNO("Failed to open %s for writing", LLDP_CONFIG_NEXT);
				break;
			}
			fprintf(fp, "configure lldp tx-interval %s\n", tx_interval);

			LY_LIST_FOR(lyd_child(node), subnode) {
    				if (!strcmp(subnode->schema->name, "port")) {
					const char *port_name = lydx_get_cattr(subnode, "name");
					const char *admin_status = lydx_get_cattr(subnode, "admin-status");

					if (strcmp(admin_status, "tx-and-rx") == 0)
						admin_status = "rx-and-tx";

					fprintf(fp, "configure ports %s lldp status %s\n", port_name, admin_status);
    				}
			}
			fclose(fp);
		}

		return SR_ERR_OK;

	case SR_EV_DONE:
		if (fexist(LLDP_CONFIG_NEXT)){
			if (erase(LLDP_CONFIG))
				ERRNO("Failed to remove old %s", LLDP_CONFIG);

			if (rename(LLDP_CONFIG_NEXT, LLDP_CONFIG))
				ERRNO("Failed switching to new %s", LLDP_CONFIG);
		}
		else
			if (erase(LLDP_CONFIG))
				ERRNO("Failed to remove old %s", LLDP_CONFIG);

		svc_change(session, event, LLDP_XPATH, "lldp", "lldpd");
		break;

	case SR_EV_ABORT:
		erase(LLDP_CONFIG_NEXT);
		break;

	default:
		break;
	}

	return SR_ERR_OK;
}

static int ttyd_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, WEB_CONSOLE_XPATH))
		return SR_ERR_OK;

	cfg = get(session, event, WEB_XPATH, &srv, "web", "console", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), ttyd, NULL);

	return put(cfg);
}

static int netbrowse_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, WEB_NETBROWSE_XPATH))
		return SR_ERR_OK;

	cfg = get(session, event, WEB_XPATH, &srv, "web", "netbrowse", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), netbrowse, NULL);
	mdns_cname(session);

	return put(cfg);
}

static int restconf_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	char *out;
	if (event != SR_EV_DONE  || !lydx_get_xpathf(diff, WEB_RESTCONF_XPATH))
		return SR_ERR_OK;

	ERROR("RESTCONF CHANGES DETECTED");
	lyd_print_mem(&out, diff, LYD_JSON,
		       LYD_PRINT_WITHSIBLINGS | LYD_PRINT_WD_ALL);
	ERROR("%s", out);
	cfg = get(session, event, WEB_XPATH, &srv, "web", "restconf", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), restconf, NULL);

	return put(cfg);
}

static int ssh_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *ssh = NULL, *listen, *host_key;
	sr_error_t rc = SR_ERR_OK;
	FILE *fp;

	if (diff && !lydx_get_xpathf(diff, SSH_XPATH))
		return SR_ERR_OK;

	switch (event) {
	case SR_EV_DONE:
		return svc_change(session, event, SSH_XPATH, "ssh", "sshd");
	case SR_EV_ENABLED:
	case SR_EV_CHANGE:
		break;

	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	ssh = lydx_get_xpathf(config, SSH_XPATH);

	if (!lydx_is_enabled(ssh, "enabled")) {
		goto out;
	}

	fp = fopen(SSHD_CONFIG_HOSTKEY, "w");
	if (!fp) {
		rc = SR_ERR_INTERNAL;
		goto out;
	}

	LY_LIST_FOR(lydx_get_child(ssh, "hostkey"), host_key) {
		const char *keyname = lyd_get_value(host_key);
		if (!keyname)
			continue;
		fprintf(fp, "HostKey %s/hostkeys/%s\n", SSH_BASE, keyname);
	}

	fclose(fp);

	fp = fopen(SSHD_CONFIG_LISTEN, "w");
	if (!fp) {
		rc = SR_ERR_INTERNAL;
		goto out;
	}

	LY_LIST_FOR(lydx_get_child(ssh, "listen"), listen) {
		const char *address, *port;
		int ipv6;

		address = lydx_get_cattr(listen, "address");
		ipv6 = !!strchr(address, ':');
		port = lydx_get_cattr(listen, "port");

		fprintf(fp, "ListenAddress %s%s%s:%s\n", ipv6 ? "[" : "", address, ipv6 ? "]" : "", port);
	}
	fclose(fp);

out:

	return rc;
}


static int web_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	int ena;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, WEB_XPATH))
		return SR_ERR_OK;

	cfg = get(session, event, WEB_XPATH, &srv, "web", NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled");
	if (ena) {
		svc_enadis(srx_enabled(session, "%s/enabled", WEB_CONSOLE_XPATH), ttyd, "ttyd");
		svc_enadis(srx_enabled(session, "%s/enabled", WEB_NETBROWSE_XPATH), netbrowse, "netbrowse");
		svc_enadis(srx_enabled(session, "%s/enabled", WEB_RESTCONF_XPATH), restconf, "restconf");
	} else {
		svc_enadis(0, ttyd, NULL);
		svc_enadis(0, netbrowse, NULL);
		svc_enadis(0, restconf, NULL);
	}

	svc_enadis(ena, web, "nginx");
	mdns_cname(session);

	return put(cfg);
}

int infix_services_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	int rc;

	rc = lldp_change(session, config, diff, event, confd);
	if (rc)
		return rc;
	rc = mdns_change(session, config, diff, event, confd); /* TODO: Depends on hostname changes */
	if (rc)
		return rc;
	rc = ssh_change(session, config, diff, event, confd); /* TODO: Depends on keystore changes*/
	if (rc)
		return rc;
	rc = web_change(session, config, diff, event, confd);
	if (rc)
		return rc;
	rc = ttyd_change(session, config, diff, event, confd);
	if (rc)
		return rc;
	rc = restconf_change(session, config, diff, event, confd);
	if (rc)
		return rc;
	rc = netbrowse_change(session, config, diff, event, confd);
	if (rc)
		return rc;
	return SR_ERR_OK;
}
