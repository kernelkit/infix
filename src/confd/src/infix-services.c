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

#define SSH_HOSTKEYS "/etc/ssh/hostkeys"
#define SSH_HOSTKEYS_NEXT SSH_HOSTKEYS"+"
 
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

static int hostname_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	return mdns_records("update", all);
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

static int lldp_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *node = NULL;
	sr_data_t *cfg;
	struct lyd_node *subnode;

	switch (event) {
	case SR_EV_ENABLED:
	case SR_EV_CHANGE:
		if (sr_get_data(session, xpath, 0, 0, 0, &cfg) || !cfg)
			break;

		node = cfg->tree;
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

		return put(cfg);

	case SR_EV_DONE:
		if (fexist(LLDP_CONFIG_NEXT)){
			if (erase(LLDP_CONFIG))
				ERRNO("Failed to remove old %s", LLDP_CONFIG);

			rename(LLDP_CONFIG_NEXT, LLDP_CONFIG);
		}
		else
			if (erase(LLDP_CONFIG))
				ERRNO("Failed to remove old %s", LLDP_CONFIG);
		
		svc_change(session, event, xpath, "lldp", "lldpd");
		break;

	case SR_EV_ABORT:
		erase(LLDP_CONFIG_NEXT);
		break;

	default:
		break;
	}

	return SR_ERR_OK;
}

static int ttyd_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;

	cfg = get(session, event, xpath, &srv, "web", "console", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), ttyd, NULL);

	return put(cfg);
}

static int netbrowse_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;

	cfg = get(session, event, xpath, &srv, "web", "netbrowse", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), netbrowse, NULL);
	mdns_cname(session);

	return put(cfg);
}

static int restconf_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *srv = NULL;
	sr_data_t *cfg;

	cfg = get(session, event, xpath, &srv, "web", "restconf", NULL);
	if (!cfg)
		return SR_ERR_OK;

	svc_enadis(lydx_is_enabled(srv, "enabled"), restconf, NULL);

	return put(cfg);
}

static int ssh_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		      const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *ssh = NULL, *listen, *host_key;
	sr_error_t rc = SR_ERR_OK;
	sr_data_t *cfg;
	FILE *fp;

	switch (event) {
	case SR_EV_DONE:
		return svc_change(session, event, xpath, "ssh", "sshd");
	case SR_EV_ENABLED:
	case SR_EV_CHANGE:
		break;

	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	if (sr_get_data(session, xpath, 0, 0, 0, &cfg) || !cfg) {
		return SR_ERR_OK;
	}
	ssh = cfg->tree;

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
	sr_release_data(cfg);

	return rc;
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
		svc_enadis(srx_enabled(session, "%s/console/enabled", xpath), ttyd, "ttyd");
		svc_enadis(srx_enabled(session, "%s/netbrowse/enabled", xpath), netbrowse, "netbrowse");
		svc_enadis(srx_enabled(session, "%s/restconf/enabled", xpath), restconf, "restconf");
	} else {
		svc_enadis(0, ttyd, NULL);
		svc_enadis(0, netbrowse, NULL);
		svc_enadis(0, restconf, NULL);
	}

	svc_enadis(ena, web, "nginx");
	mdns_cname(session);

	return put(cfg);
}

/* Store SSH public/private keys */
static int change_keystore_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *module_name,
		     const char *xpath, sr_event_t event, uint32_t request_id, void *_)
{
	int rc = SR_ERR_OK;
	sr_data_t *cfg;
	struct lyd_node *changes, *change;

	switch (event) {
	case SR_EV_CHANGE:
	case SR_EV_ENABLED:
		break;
	case SR_EV_ABORT:
		rmrf(SSH_HOSTKEYS_NEXT);
		return SR_ERR_OK;
	case SR_EV_DONE:
		if(fexist(SSH_HOSTKEYS_NEXT)) {
			if(rmrf(SSH_HOSTKEYS)) {
				ERRNO("Failed to remove old SSH hostkeys: %d", errno);
			}
			rename(SSH_HOSTKEYS_NEXT, SSH_HOSTKEYS);
			svc_change(session, event, "/infix-services:ssh", "ssh", "sshd");
		}
		return SR_ERR_OK;

	default:
		return SR_ERR_OK;
	}

	if (sr_get_data(session, "/ietf-keystore:keystore/asymmetric-keys//.", 0, 0, 0, &cfg) || !cfg) {
		return SR_ERR_OK;
	}
	changes = lydx_get_descendant(cfg->tree, "keystore", "asymmetric-keys", "asymmetric-key", NULL);

	LYX_LIST_FOR_EACH(changes, change, "asymmetric-key") {
		const char *name, *private_key_type, *public_key_type;
		const char *private_key, *public_key;

		name = lydx_get_cattr(change, "name");
		private_key_type = lydx_get_cattr(change, "private-key-format");
		public_key_type = lydx_get_cattr(change, "public-key-format");

		if (strcmp(private_key_type, "ietf-crypto-types:rsa-private-key-format")) {
			INFO("Private key %s is not of SSH type", name);
			continue;
		}

		if (strcmp(public_key_type, "ietf-crypto-types:ssh-public-key-format")) {
			INFO("Public key %s is not of SSH type", name);
			continue;
		}
		private_key = lydx_get_cattr(change, "cleartext-private-key");
		public_key = lydx_get_cattr(change, "public-key");
		mkdir(SSH_HOSTKEYS_NEXT, 0600);
		if(systemf("/usr/libexec/infix/mksshkey %s %s %s %s", name, SSH_HOSTKEYS_NEXT, public_key, private_key))
			rc = SR_ERR_INTERNAL;
       }

	sr_release_data(cfg);

	return rc;
}

int infix_services_init(struct confd *confd)
{
	int rc;

	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:mdns",
			0, mdns_change, confd, &confd->sub);
	REGISTER_MONITOR(confd->session, "ietf-system", "/ietf-system:system/hostname",
			 0, hostname_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:ssh",
			0, ssh_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:web",
			0, web_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:web/infix-services:console",
			0, ttyd_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:web/infix-services:netbrowse",
			0, netbrowse_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "infix-services", "/infix-services:web/infix-services:restconf",
			0, restconf_change, confd, &confd->sub);
	REGISTER_CHANGE(confd->session, "ieee802-dot1ab-lldp", "/ieee802-dot1ab-lldp:lldp",
			0, lldp_change, confd, &confd->sub);

        /* Store SSH keys */
	REGISTER_CHANGE(confd->session, "ietf-keystore", "/ietf-keystore:keystore//.",
			0, change_keystore_cb, confd, &confd->sub);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
