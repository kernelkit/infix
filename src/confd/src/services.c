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

#include "core.h"
#include <sysrepo_types.h>

#define GENERATE_ENUM(ENUM)      ENUM,
#define GENERATE_STRING(STRING) #STRING,

#define NGINX_SSL_CONF "/etc/nginx/ssl.conf"
#define AVAHI_SVC_PATH "/etc/avahi/services"

#define LLDP_CONFIG "/etc/lldpd.d/confd.conf"
#define LLDP_CONFIG_NEXT LLDP_CONFIG"+"

enum mdns_cmd { MDNS_ADD, MDNS_DELETE, MDNS_UPDATE };

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
	{ web,      "https",    "_https._tcp",        443, "%h Web",     "adminurl=https://%s.local" },
	{ ttyd,     "ttyd",     "_https._tcp",        443, "%h Console", "adminurl=https://%s.local:7681" },
	{ web,      "http",     "_http._tcp",          80, "%h Web",     "adminurl=http://%s.local" },
	{ netconf,  "netconf",  "_netconf-ssh._tcp",  830, "%h",         NULL },
	{ restconf, "restconf", "_restconf-tls._tcp", 443, "%h",         NULL },
	{ ssh,      "sftp-ssh", "_sftp-ssh._tcp",      22, "%h",         NULL },
	{ ssh,      "ssh",      "_ssh._tcp",           22, "%h",         NULL },
};

static const char *jgets(json_t *obj, const char *key)
{
	json_t *val = json_object_get(obj, key);

	return (val && !json_is_null(val)) ? json_string_value(val) : NULL;
}

/* Write str to fp with XML special characters escaped. */
static void xml_escape(FILE *fp, const char *str)
{
	for (; *str; str++) {
		switch (*str) {
		case '&':  fputs("&amp;",  fp); break;
		case '<':  fputs("&lt;",   fp); break;
		case '>':  fputs("&gt;",   fp); break;
		case '"':  fputs("&quot;", fp); break;
		case ';':                       break; /* avahi txt-record separator */
		default:   fputc(*str,     fp); break;
		}
	}
}

static void write_txt(FILE *fp, const char *key, const char *val)
{
	if (!val)
		return;
	fputs("    <txt-record>", fp);
	xml_escape(fp, key);
	fputc('=', fp);
	xml_escape(fp, val);
	fputs("</txt-record>\n", fp);
}

/*
 * On hostname changes we need to update the mDNS records, in particular
 * the ones advertising an adminurl (standarized by Apple), because they
 * include the fqdn in the URL.
 *
 * XXX: when the web managment interface is in place we can change the
 *      adminurl to include 'admin@%s.local' to pre-populate the default
 *      username in the login dialog.
 */
static int mdns_records(int cmd, svc type)
{
	char hostname[MAXHOSTNAMELEN + 1];
	const char *vendor, *product, *serial, *mac;
	const char *vn, *on, *ov;

	if (gethostname(hostname, sizeof(hostname))) {
		ERRNO("failed getting system hostname");
		return SR_ERR_SYS;
	}

	vendor  = jgets(confd.root, "vendor");
	product = jgets(confd.root, "product-name");
	serial  = jgets(confd.root, "serial-number");
	mac     = jgets(confd.root, "mac-address");

	vn = fgetkey("/etc/os-release", "VENDOR_NAME");
	on = fgetkey("/etc/os-release", "NAME");
	ov = fgetkey("/etc/os-release", "VERSION_ID");

	for (size_t i = 0; i < NELEMS(services); i++) {
		struct mdns_svc *srv = &services[i];
		FILE *fp;

		if (type != all && srv->svc != type)
			continue;

		if (cmd == MDNS_DELETE) {
			erasef(AVAHI_SVC_PATH "/%s.service", srv->name);
			continue;
		}

		if (cmd == MDNS_UPDATE && !fexistf(AVAHI_SVC_PATH "/%s.service", srv->name))
			continue;

		fp = fopenf("w", AVAHI_SVC_PATH "/%s.service", srv->name);
		if (!fp) {
			ERRNO("failed creating %s.service", srv->name);
			continue;
		}

		fprintf(fp,
			"<?xml version=\"1.0\" standalone='no'?>\n"
			"<!DOCTYPE service-group SYSTEM \"avahi-service.dtd\">\n"
			"<service-group>\n"
			"  <name replace-wildcards=\"yes\">%s</name>\n"
			"  <service>\n"
			"    <type>%s</type>\n"
			"    <port>%d</port>\n"
			"    <txt-record>vv=1</txt-record>\n",
			srv->desc, srv->type, srv->port);
		write_txt(fp, "vendor",   vendor);
		write_txt(fp, "product",  product);
		write_txt(fp, "serial",   serial);
		write_txt(fp, "deviceid", mac);
		write_txt(fp, "vn",       vn);
		write_txt(fp, "on",       on);
		write_txt(fp, "ov",       ov);

		if (srv->text) {
			char txt[256];

			snprintf(txt, sizeof(txt), srv->text, hostname);
			fputs("    <txt-record>", fp);
			xml_escape(fp, txt);
			fputs("</txt-record>\n", fp);
		}

		fprintf(fp,
			"  </service>\n"
			"</service-group>\n");
		fclose(fp);
	}

	/* Always-on records tied to mDNS being active, not a specific service */
	if (type == all) {
		if (cmd == MDNS_DELETE) {
			erasef(AVAHI_SVC_PATH "/workstation.service");
			erasef(AVAHI_SVC_PATH "/device-info.service");
		} else {
			FILE *fp;

			fp = fopenf("w", AVAHI_SVC_PATH "/workstation.service");
			if (fp) {
				fprintf(fp,
					"<?xml version=\"1.0\" standalone='no'?>\n"
					"<!DOCTYPE service-group SYSTEM \"avahi-service.dtd\">\n"
					"<service-group>\n"
					"  <name replace-wildcards=\"yes\">%%h [%s]</name>\n"
					"  <service>\n"
					"    <type>_workstation._tcp</type>\n"
					"    <port>9</port>\n"
					"  </service>\n"
					"</service-group>\n",
					mac ?: "");
				fclose(fp);
			} else {
				ERRNO("failed creating workstation.service");
			}

			/* TODO: Use device-info YANG model for Apple-compatible model string */
			fp = fopenf("w", AVAHI_SVC_PATH "/device-info.service");
			if (fp) {
				fprintf(fp,
					"<?xml version=\"1.0\" standalone='no'?>\n"
					"<!DOCTYPE service-group SYSTEM \"avahi-service.dtd\">\n"
					"<service-group>\n"
					"  <name replace-wildcards=\"yes\">%%h</name>\n"
					"  <service>\n"
					"    <type>_device-info._tcp</type>\n"
					"    <port>0</port>\n");
				write_txt(fp, "model", product);
				fprintf(fp,
					"  </service>\n"
					"</service-group>\n");
				fclose(fp);
			} else {
				ERRNO("failed creating device-info.service");
			}
		}
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

/*
 * Enable or disable a named service: manage nginx symlinks and mDNS
 * records, then start or stop via initctl.  Does NOT touch (restart)
 * the service -- call finit_reload() separately when only config
 * changes and the service is already running.
 */
static void svc_enable(int ena, svc type, const char *svcname)
{
	if (!svcname)
		svcname = name[type];

	if (fexistf("/etc/nginx/available/%s.conf", svcname)) {
		char src[256], dst[256];

		snprintf(dst, sizeof(dst), "/etc/nginx/enabled/%s.conf", svcname);
		if (ena) {
			snprintf(src, sizeof(src), "../available/%s.conf", svcname);
			erase(dst);
			symlink(src, dst);
		} else {
			erase(dst);
		}
	}
	if (fexistf("/etc/nginx/%s.app", svcname)) {
		char src[256], dst[256];

		snprintf(dst, sizeof(dst), "/etc/nginx/app/%s.conf", svcname);
		if (ena) {
			snprintf(src, sizeof(src), "../%s.app", svcname);
			erase(dst);
			symlink(src, dst);
		} else {
			erase(dst);
		}
	}

	ena ? finit_enable(svcname) : finit_disable(svcname);

	if (type != none)
		mdns_records(ena ? MDNS_ADD : MDNS_DELETE, type);
}

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

static void mdns_conf(struct confd *confd, struct lyd_node *cfg)
{
	char hname[HOST_NAME_MAX + 1];
	const char *hostname;
	const char *fmt;
	struct lyd_node *ctx;
	FILE *fp;

	fmt = lydx_get_cattr(cfg, "hostname");   /* "%h" when unset (YANG default) */
	if (!hostnamefmt(confd, fmt, hname, sizeof(hname), NULL, 0))
		hostname = hname;
	else
		hostname = fgetkey("/etc/os-release", "DEFAULT_HOSTNAME");

	fp = fopen(AVAHI_CONF, "w");
	if (!fp) {
		ERRNO("failed creating %s", AVAHI_CONF);
		return;
	}

	fprintf(fp, "# Generated by Infix confd\n"
		"[server]\n"
		"host-name=%s\n"
		"domain-name=%s\n"
		"use-ipv4=yes\n"
		"use-ipv6=yes\n", hostname, lydx_get_cattr(cfg, "domain"));

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
		fprintf(fp, "enable-reflector=%s\n", lydx_is_enabled(ctx, "enabled") ? "yes" : "no");
		fput_list(fp, ctx, "service-filter", "reflect-filters=");
	}

	fprintf(fp, "\n[rlimits]\n");
	/* nop */

	fclose(fp);
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
		mdns_conf(confd, srv);
		mdns_records(MDNS_UPDATE, all);
	}

	if (lydx_get_xpathf(diff, MDNS_XPATH "/enabled")) {
		svc_enable(ena, none, "avahi");
		svc_enable(ena, none, "mdns-alias");
	}
	if (ena)
		finit_reload("avahi");

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

		{
			struct lyd_node *lldp = lydx_get_xpathf(config, LLDP_XPATH);
			int lldp_ena = lydx_is_enabled(lldp, "enabled");

			if (lydx_get_xpathf(diff, LLDP_XPATH "/enabled"))
				lldp_ena ? finit_enable("lldpd") : finit_disable("lldpd");
			else if (lldp_ena)
				finit_reload("lldpd");
		}
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
	int ena;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, WEB_CONSOLE_XPATH "/enabled"))
		return SR_ERR_OK;

	cfg = get(session, event, WEB_XPATH, &srv, "web", "console", NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled") &&
	      lydx_is_enabled(lydx_get_xpathf(config, WEB_XPATH), "enabled");
	svc_enable(ena, ttyd, NULL);
	finit_reload("nginx");

	return put(cfg);
}

static void mdns_alias_conf(int ena)
{
	FILE *fp = fopen("/etc/default/mdns-alias", "w");

	if (fp) {
		fprintf(fp, "MDNS_ALIAS_ARGS=\"%s\"\n", ena ? "network.local" : "");
		fclose(fp);
	} else {
		ERRNO("failed updating mDNS aliases");
	}
}

static int netbrowse_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
			    sr_event_t event, struct confd *confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	int ena;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, WEB_NETBROWSE_XPATH "/enabled"))
		return SR_ERR_OK;

	cfg = get(session, event, WEB_XPATH, &srv, "web", "netbrowse", NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled") &&
	      lydx_is_enabled(lydx_get_xpathf(config, WEB_XPATH), "enabled");
	svc_enable(ena, netbrowse, NULL);
	mdns_alias_conf(ena);
	finit_reload("nginx");
	finit_reload("mdns-alias");

	return put(cfg);
}

static int restconf_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;
	int ena;

	if (event != SR_EV_DONE || !lydx_get_xpathf(diff, WEB_RESTCONF_XPATH "/enabled"))
		return SR_ERR_OK;

	cfg = get(session, event, WEB_XPATH, &srv, "web", "restconf", NULL);
	if (!cfg)
		return SR_ERR_OK;

	ena = lydx_is_enabled(srv, "enabled") &&
	      lydx_is_enabled(lydx_get_xpathf(config, WEB_XPATH), "enabled");
	svc_enable(ena, restconf, "restconf");
	finit_reload("nginx");

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
		{
			struct lyd_node *ssh = lydx_get_xpathf(config, SSH_XPATH);
			int ssh_ena = lydx_is_enabled(ssh, "enabled");

			if (lydx_get_xpathf(diff, SSH_XPATH "/enabled"))
				ssh_ena ? finit_enable("sshd") : finit_disable("sshd");
			else if (ssh_ena)
				finit_reload("sshd");
		}
		return SR_ERR_OK;
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


static void web_ssl_conf(struct lyd_node *srv, struct lyd_node *config)
{
	const char *keyref, *certname = "self-signed";
	struct lyd_node *key, *certs;
	FILE *fp;

	keyref = lydx_get_cattr(srv, "certificate");
	if (!keyref)
		keyref = "gencert";

	key = lydx_get_xpathf(config, "/ietf-keystore:keystore/asymmetric-keys"
			       "/asymmetric-key[name='%s']", keyref);
	if (key) {
		certs = lydx_get_descendant(lyd_child(key), "certificates", "certificate", NULL);
		if (certs) {
			const char *name = lydx_get_cattr(certs, "name");

			if (name && *name)
				certname = name;
		}
	}

	fp = fopen(NGINX_SSL_CONF, "w");
	if (!fp) {
		ERRNO("failed creating %s", NGINX_SSL_CONF);
		return;
	}

	fprintf(fp,
		"ssl_certificate      %s/%s.crt;\n"
		"ssl_certificate_key  %s/%s.key;\n"
		"\n"
		"ssl_protocols        TLSv1.3 TLSv1.2;\n"
		"ssl_ciphers          HIGH:!aNULL:!MD5;\n"
		"ssl_prefer_server_ciphers  on;\n"
		"\n"
		"ssl_session_cache    shared:SSL:1m;\n"
		"ssl_session_timeout  5m;\n",
		SSL_CERT_DIR, certname, SSL_KEY_DIR, certname);
	fclose(fp);
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

	/* Certificate changed: regenerate ssl.conf and reload nginx */
	if (lydx_get_xpathf(diff, WEB_XPATH "/certificate")) {
		web_ssl_conf(srv, config);
		finit_reload("nginx");
	}

	/* Web master on/off: propagate to nginx and all sub-services */
	if (lydx_get_xpathf(diff, WEB_XPATH "/enabled")) {
		int nb_ena = ena && lydx_is_enabled(lydx_get_xpathf(config, WEB_NETBROWSE_XPATH), "enabled");

		svc_enable(ena && lydx_is_enabled(lydx_get_xpathf(config, WEB_CONSOLE_XPATH), "enabled"),
			   ttyd, "ttyd");
		svc_enable(nb_ena, netbrowse, "netbrowse");
		svc_enable(ena && lydx_is_enabled(lydx_get_xpathf(config, WEB_RESTCONF_XPATH), "enabled"),
			   restconf, "restconf");
		svc_enable(ena, web, "nginx");
		mdns_alias_conf(nb_ena);
		finit_reload("mdns-alias");
	}

	return put(cfg);
}

int services_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff, sr_event_t event, struct confd *confd)
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
