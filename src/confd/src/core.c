/* SPDX-License-Identifier: BSD-3-Clause */

#include <srx/srx_val.h>
#include <srx/common.h>
#include <srx/lyx.h>

#include <ctype.h>
#include <grp.h>
#include <net/if.h>
#include <sys/stat.h>
#include <unistd.h>

#include "interfaces.h"
#include "core.h"

struct confd confd;

/* Same rules as the YANG interface-name type, for names from outside the model */
bool valid_ifname(const char *name)
{
	size_t len = strnlen(name, IFNAMSIZ);

	if (!len || len >= IFNAMSIZ || (!isalnum(*name) && *name != '_'))
		return false;
	for (; *name; name++) {
		if (!isalnum(*name) && !strchr("._:+-", *name))
			return false;
	}

	return true;
}

/*
 * Write str to fp in double quotes, with '"' and '\' escaped, for
 * config files that unescape quoted values, e.g. syslog.conf, dnsmasq.
 */
int fputq(const char *str, FILE *fp)
{
	if (fputc('"', fp) == EOF)
		return EOF;
	for (; *str; str++) {
		if ((*str == '"' || *str == '\\') && fputc('\\', fp) == EOF)
			return EOF;
		if (fputc(*str, fp) == EOF)
			return EOF;
	}

	return fputc('"', fp) == EOF ? EOF : 0;
}

/*
 * confd runs with umask 0027, so generated files are readable by root
 * only.  Use these for files read by a daemon running as another user,
 * or for the few files that must stay world readable.
 */
FILE *fopenp(const char *path, mode_t mode, const char *group)
{
	gid_t gid = 0;
	FILE *fp;

	if (group) {
		struct group *gr = getgrnam(group);

		if (gr)
			gid = gr->gr_gid;
		else
			ERROR("No group %s, %s will be readable by root only", group, path);
	}

	fp = fopen(path, "w");
	if (!fp)
		return NULL;

	if (fchown(fileno(fp), 0, gid) || fchmod(fileno(fp), mode))
		ERRNO("Failed setting owner/mode on %s", path);

	return fp;
}

FILE *fopenfp(mode_t mode, const char *group, const char *fmt, ...)
{
	va_list ap;
	char *path;
	FILE *fp;
	int len;

	va_start(ap, fmt);
	len = vasprintf(&path, fmt, ap);
	va_end(ap);
	if (len < 0)
		return NULL;

	fp = fopenp(path, mode, group);
	free(path);

	return fp;
}

/* The event session runs as confd itself, the caller is only known
 * from the originator: netopeer2 pushes the NETCONF session id and
 * then the username as data, rousette and the rpc tool name the
 * session after the user */
const char *rpc_user(sr_session_ctx_t *session, const char **via)
{
	const char *orig = sr_session_get_orig_name(session);
	const void *data;
	uint32_t size;

	*via = orig && orig[0] ? orig : "local session";

	if (orig && !strcmp(orig, "netopeer2") &&
	    !sr_session_get_orig_data(session, 1, &size, &data) && size)
		return data;

	return NULL;
}

/*
 * Touch a Finit service .conf file to schedule a synchronized reload.
 * Equivalent to 'initctl touch <svc>' but without the fork+exec overhead.
 * The service name should be given without the .conf suffix.
 */
int finit_reload(const char *svc)
{
	char path[256];

	/* Prefer the enabled/ symlink -- that's what Finit watches */
	snprintf(path, sizeof(path), FINIT_RCSD "/enabled/%s.conf", svc);
	if (!utimensat(AT_FDCWD, path, NULL, AT_SYMLINK_NOFOLLOW))
		return 0;

	snprintf(path, sizeof(path), FINIT_RCSD "/available/%s.conf", svc);
	if (!utimensat(AT_FDCWD, path, NULL, AT_SYMLINK_NOFOLLOW))
		return 0;

	ERRNO("failed marking %s for reload", svc);
	return -1;
}

int finit_reloadf(const char *fmt, ...)
{
	char svc[64];
	va_list ap;

	va_start(ap, fmt);
	vsnprintf(svc, sizeof(svc), fmt, ap);
	va_end(ap);

	return finit_reload(svc);
}

int finit_enable(const char *svc)
{
	char src[256], dst[256];
	const char *at;

	snprintf(src, sizeof(src), FINIT_RCSD "/available/%s.conf", svc);
	if (!fexist(src)) {
		/* Template instance (e.g. container@foo): point to base template */
		at = strchr(svc, '@');
		if (at)
			snprintf(src, sizeof(src), FINIT_RCSD "/available/%.*s@.conf",
				 (int)(at - svc), svc);
	}

	if (!fexist(src)) {
		/* Optional service not part of this image, avoid dangling symlink */
		INFO("%s is not available in this image, cannot enable", svc);
		return 0;
	}

	snprintf(dst, sizeof(dst), FINIT_RCSD "/enabled/%s.conf", svc);
	if (symlink(src, dst) && errno != EEXIST) {
		ERRNO("failed enabling %s", svc);
		return -1;
	}
	return 0;
}

int finit_disable(const char *svc)
{
	char path[256];

	snprintf(path, sizeof(path), FINIT_RCSD "/enabled/%s.conf", svc);
	if (remove(path) && errno != ENOENT) {
		ERRNO("failed disabling %s", svc);
		return -1;
	}
	return 0;
}

int finit_delete(const char *svc)
{
	char path[256];

	snprintf(path, sizeof(path), FINIT_RCSD "/enabled/%s.conf", svc);
	if (remove(path) && errno != ENOENT) {
		ERRNO("failed removing enabled symlink for %s", svc);
		return -1;
	}

	snprintf(path, sizeof(path), FINIT_RCSD "/available/%s.conf", svc);
	if (remove(path) && errno != ENOENT) {
		ERRNO("failed removing available conf for %s", svc);
		return -1;
	}

	return 0;
}

int finit_deletef(const char *fmt, ...)
{
	char svc[64];
	va_list ap;

	va_start(ap, fmt);
	vsnprintf(svc, sizeof(svc), fmt, ap);
	va_end(ap);

	return finit_delete(svc);
}

int finit_enablef(const char *fmt, ...)
{
	char svc[64];
	va_list ap;

	va_start(ap, fmt);
	vsnprintf(svc, sizeof(svc), fmt, ap);
	va_end(ap);

	return finit_enable(svc);
}

int finit_disablef(const char *fmt, ...)
{
	char svc[64];
	va_list ap;

	va_start(ap, fmt);
	vsnprintf(svc, sizeof(svc), fmt, ap);
	va_end(ap);

	return finit_disable(svc);
}


/*
 * After a migration the startup datastore has the old version of the file
 * until saved, so stamp every write after bootstrap, issue #1637.
 */
static int startup_version(sr_session_ctx_t *session, uint32_t sub_id, const char *model,
			   const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	if (event != SR_EV_UPDATE || systemf("runlevel >/dev/null 2>&1"))
		return SR_ERR_OK;

	return meta_set_version(session);
}

static int startup_save(sr_session_ctx_t *session, uint32_t sub_id, const char *model,
			const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	/* skip in bootstrap, triggered by load script to initialize startup datastore */
	if (systemf("runlevel >/dev/null 2>&1"))
		return SR_ERR_OK;

	if (systemf("copy -f startup /cfg/startup-config.cfg"))
		return SR_ERR_SYS;

	return SR_ERR_OK;
}

static int change_cb(sr_session_ctx_t *session, uint32_t sub_id, const char *model_name,
		     const char *xpath, sr_event_t event, uint32_t request_id, void *_confd)
{
	struct lyd_node *diff = NULL, *config = NULL;
	static sr_event_t last_event = -1;
	struct confd *confd = _confd;
	static uint32_t last_request = 0;
	sr_data_t *cfg = NULL;
	int rc = SR_ERR_OK;

	if (request_id == last_request && last_event == event)
		return SR_ERR_OK;
	last_request = request_id;
	last_event = event;

	if (event == SR_EV_UPDATE || event == SR_EV_CHANGE || event == SR_EV_DONE) {
		rc = srx_get_diff(session, &diff);
		if (rc != SR_ERR_OK) {
			ERROR("Failed to get diff: %d", rc);
			return rc;
		}
		rc = sr_get_data(session, "//.", 0, 0, 0, &cfg);
		if (rc || !cfg)
			goto free_diff;

		config = cfg->tree;
		if (dep_resolve(&diff, config)) {
			rc = SR_ERR_INTERNAL;
			goto free_diff;
		}
	}

	/* ietf-hardware, before the interfaces: netdevs behind a USB port
	 * appear only once the port is unlocked, and a modem netdev gets
	 * its name from the udev rules written here. */
	if ((rc = hardware_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-interfaces */
	if ((rc = interfaces_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-hardware WiFi, after the interfaces: hostapd binds to the
	 * netdevs the interfaces have just created or recreated. */
	if ((rc = hardware_wifi_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-dhcp-client*/
	if ((rc = dhcp_client_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-dhcpv6-client*/
	if ((rc = dhcpv6_client_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-keystore */
	if ((rc = keystore_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-ntp */
	if ((rc = ntp_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ieee1588-ptp-tt */
	if ((rc = ptp_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-services */
	if ((rc = services_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-alarms */
	if ((rc = alarm_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-snmp */
	if ((rc = snmp_change(session, config, diff, event, confd)))
		goto free_diff;

	/* ietf-syslog*/
	if ((rc = syslog_change(session, config, diff, event, confd)))
		goto free_diff;

	/* NOTE!
	 * ietf-system, keep last: /system/advanced/defaults replaces whole
	 * files under /etc/default, so it has to run after every handler
	 * that writes one of its own.
	 */
	if ((rc = system_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-schedule */
	if ((rc = schedule_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-containers */
#ifdef CONTAINERS
	if ((rc = containers_change(session, config, diff, event, confd)))
		goto free_diff;
#endif

	/* ietf-routing */
	if ((rc = routing_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-dhcp-server */
	if ((rc = dhcp_server_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-firewall */
	if ((rc = firewall_change(session, config, diff, event, confd)))
		goto free_diff;

	/* infix-meta */
	if ((rc = meta_change_cb(session, config, diff, event, confd)))
		goto free_diff;

	/*
	 * Manage chronyd service enable/disable state.  Must be done
	 * after both ietf-system:ntp and ietf-ntp have are done.
	 */
	if (event == SR_EV_DONE && config) {
		bool client = false;
		bool server = false;

		client = srx_enabled(session, "/ietf-system:system/ntp/enabled");
		server = lydx_get_xpathf(config, "/ietf-ntp:ntp") != NULL;

		(client || server) ? finit_enable("chronyd") : finit_disable("chronyd");
	}

	if (cfg) {
		sr_release_data(cfg);
		cfg = NULL;
	}

	if (event == SR_EV_DONE) {
		/* skip reload in bootstrap, implicit reload in runlevel change */
		if (systemf("runlevel >/dev/null 2>&1"))
			goto free_diff;

		/* Polled operational data catches up with the new config */
		finit_reload("yangerd");

		if (systemf("initctl -b reload")) {
			EMERG("initctl reload: failed applying new configuration!");
			rc = SR_ERR_SYS;
			goto free_diff;
		}

		AUDIT("The new configuration has been applied.");
	}

free_diff:
	if (cfg)
		sr_release_data(cfg);
	lyd_free_tree(diff);
	return rc;
}

static inline int subscribe_model(char *model, struct confd *confd, int flags)
{
	return  sr_module_change_subscribe(confd->session, model, "//.", change_cb, confd,
					   CB_PRIO_PRIMARY, SR_SUBSCR_CHANGE_ALL_MODULES |
					   SR_SUBSCR_NO_THREAD | flags, &confd->sub) ||
		sr_module_change_subscribe(confd->startup, model, "//.", startup_save, NULL,
					   CB_PRIO_PASSIVE, SR_SUBSCR_CHANGE_ALL_MODULES |
					   SR_SUBSCR_PASSIVE | SR_SUBSCR_NO_THREAD, &confd->sub);
}

int sr_plugin_init_cb(sr_session_ctx_t *session, void **priv)
{
	int log_opts = LOG_PID | LOG_NDELAY;
	int rc = SR_ERR_SYS;
	const char *env;
	json_t *product;

	/* Convert into command line option+SIGUSR1 when converting to standalone confd */
	env = getenv("DEBUG");
	if (env) {
		log_opts |= LOG_PERROR;
		debug = 1;
	}

	openlog("confd", log_opts, LOG_DAEMON);

	/* Save context with default running config datastore for all our models */
	*priv = (void *)&confd;
	confd.session = session;
	confd.conn = sr_session_get_connection(session);
	confd.sub = NULL;
	confd.fsub = NULL;

	if (!confd.conn)
		goto err;

	/* The startup datastore is used for the startup_save() hook */
	rc = sr_session_start(confd.conn, SR_DS_STARTUP, &confd.startup);
	if (rc)
		goto err;

	/* Used by, e.g., ietf-interfaces, to update interfaces types */
	rc = sr_session_start(confd.conn, SR_DS_CANDIDATE, &confd.cand);
	if (rc)
		goto err;

	confd.root = json_load_file("/run/system.json", 0, NULL);
	if (!confd.root) {
		ERROR("failed loading /run/system.json, system probe incomplete");
		rc = SR_ERR_SYS;
		goto err;
	}

	/* Optional hardware quirks for network interfaces: built-in ones
	 * for pluggable hardware, overridden by those of the running board.
	 */
	confd.ifquirks = json_load_file("/etc/interface-quirks.json", 0, NULL);
	product = json_load_file("/etc/product/interface-quirks.json", 0, NULL);
	if (!confd.ifquirks) {
		confd.ifquirks = product;
	} else if (product) {
		json_object_update(confd.ifquirks, product);
		json_decref(product);
	}

	rc = subscribe_model("ietf-interfaces", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-interfaces");
		goto err;
	}
	rc = subscribe_model("ietf-netconf-acm", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-netconf-acm");
		goto err;
	}
	rc = subscribe_model("ietf-ntp", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-ntp");
		goto err;
	}
	rc = subscribe_model("infix-dhcp-client", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-dhcp-client");
		goto err;
	}
	rc = subscribe_model("ietf-keystore", &confd, SR_SUBSCR_UPDATE);
	if (rc) {
		ERROR("Failed to subscribe to ietf-keystore");
		goto err;
	}
	rc = subscribe_model("infix-services", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-services");
		goto err;
	}
	rc = subscribe_model("ietf-system", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-system");
		goto err;
	}
	rc = subscribe_model("ietf-syslog", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-syslog");
		goto err;
	}
	rc = subscribe_model("ieee802-dot1ab-lldp", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ieee802-dot1ab-lldp");
		goto err;
	}
#ifdef CONTAINERS
	rc = subscribe_model("infix-containers", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-containers");
		goto err;
	}
#endif
	rc = subscribe_model("infix-dhcp-server", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-dhcp-server");
		goto err;
	}
	rc = subscribe_model("ietf-routing", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-routing");
		goto err;
	}
	rc = subscribe_model("ietf-hardware", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-hardware");
		goto err;
	}
	rc = subscribe_model("infix-schedule", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-schedule");
		goto err;
	}
	rc = subscribe_model("ietf-alarms", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-alarms");
		goto err;
	}
	rc = subscribe_model("infix-firewall", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to infix-firewall");
		goto err;
	}
	rc = subscribe_model("infix-meta", &confd, SR_SUBSCR_UPDATE);
	if (rc) {
		ERROR("Failed to subscribe to infix-meta");
		goto err;
	}
	rc = sr_module_change_subscribe(confd.startup, "infix-meta", "//.", startup_version, NULL,
					CB_PRIO_PRIMARY, SR_SUBSCR_UPDATE | SR_SUBSCR_NO_THREAD, &confd.sub);
	if (rc) {
		ERROR("Failed to subscribe to infix-meta in startup");
		goto err;
	}
	rc = subscribe_model("ieee1588-ptp-tt", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ieee1588-ptp-tt");
		goto err;
	}
#ifdef HAVE_SNMP
	rc = subscribe_model("ietf-snmp", &confd, 0);
	if (rc) {
		ERROR("Failed to subscribe to ietf-snmp");
		goto err;
	}
#endif

	rc = system_rpc_init(&confd);
	if (rc)
		goto err;
#ifdef CONTAINERS
	rc = containers_rpc_init(&confd);
	if (rc)
		goto err;
#endif
	rc = dhcp_server_rpc_init(&confd);
	if (rc)
		goto err;

	rc = factory_rpc_init(&confd);
	if (rc)
		goto err;

	rc = factory_default_rpc_init(&confd);
	if (rc)
		goto err;

	rc = firewall_rpc_init(&confd);
	if (rc)
		goto err;

	rc = system_sw_rpc_init(&confd);
	if (rc)
		goto err;

	rc = hardware_rpc_init(&confd);
	if (rc)
		goto err;

	rc = bridge_rpc_init(&confd);
	if (rc)
		goto err;

	rc = ethernet_rpc_init(&confd);
	if (rc)
		goto err;

	rc = routing_rpc_init(&confd);
	if (rc)
		goto err;

	rc = support_rpc_init(&confd);
	if (rc)
		goto err;

	rc = locate_rpc_init(&confd);
	if (rc)
		goto err;

	rc = syslog_rpc_init(&confd);
	if (rc)
		goto err;

	rc = alarm_rpc_init(&confd);
	if (rc)
		goto err;

	/* Candidate infer configurations */
	rc = interfaces_cand_init(&confd);
	if (rc)
		goto err;

	rc = hardware_candidate_init(&confd);
	if (rc)
		goto err;

	rc = firewall_candidate_init(&confd);
	if (rc)
		goto err;

	rc = dhcp_server_candidate_init(&confd);
	if (rc)
		goto err;

	rc = ntp_candidate_init(&confd);
	if (rc)
		goto err;
	rc = modem_init(&confd);
	if (rc)
		goto err;

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(rc));
	if (confd.root)
		json_decref(confd.root);
	sr_unsubscribe(confd.sub);
	sr_unsubscribe(confd.fsub);

	return rc;
}

void confd_get_subscriptions(void *priv, sr_subscription_ctx_t **out_sub,
			     sr_subscription_ctx_t **out_fsub)
{
	struct confd *c = (struct confd *)priv;

	*out_sub  = c->sub;
	*out_fsub = c->fsub;
}

void sr_plugin_cleanup_cb(sr_session_ctx_t *session, void *priv)
{
	struct confd *ptr = (struct confd *)priv;

	sr_unsubscribe(ptr->sub);
	sr_unsubscribe(ptr->fsub);
	json_decref(ptr->root);
	closelog();
}
