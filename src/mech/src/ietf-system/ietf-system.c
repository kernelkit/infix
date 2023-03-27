/* SPDX-License-Identifier: Apache-2.0 */

#include <augeas.h>
#include <ctype.h>
#include <errno.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <syslog.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

#include "common.h"

#define XMLNS "urn:ietf:params:xml:ns:yang:ietf-system"

static augeas *aug;
static char   *ver = NULL;
static char   *rel = NULL;
static char   *sys = NULL;
static char   *os  = NULL;

/**
 * Like system(), but takes a formatted string as argument.
 * @param fmt  printf style format list to command to run
 *
 * This system() wrapper greatly simplifies operations that usually
 * consist of composing a command from parts into a dynamic buffer
 * before calling it.  The return value from system() is also parsed,
 * checking for proper exit and signals.
 *
 * @returns If the command exits normally, the return code of the command
 * is returned.  Otherwise, if the command is signalled, the return code
 * is -1 and @a errno is set to @c EINTR.
 */
static int run(const char *fmt, ...)
{
	va_list ap;
	char *cmd;
	int len;
	int rc;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap);
	va_end(ap);

	cmd = alloca(++len);
	if (!cmd) {
		errno = ENOMEM;
		return -1;
	}

	va_start(ap, fmt);
	vsnprintf(cmd, len, fmt, ap);
	va_end(ap);

	rc = system(cmd);
	if (rc == -1)
		return -1;

	if (WIFEXITED(rc)) {
		errno = 0;
		rc = WEXITSTATUS(rc);
	} else if (WIFSIGNALED(rc)) {
		errno = EINTR;
		rc = -1;
	}

//	clicon_debug(LOG_ERR, "ietf-system:run(%s) => %d", cmd, rc);

	return rc;
}

static bool is_true(cxobj *xp, char *name)
{
	cxobj *obj;

	if (!xp || !name)
		return false;

	obj = xml_find(xp, name);
	if (obj && !strcmp(xml_body(obj), "true"))
		return true;

	return false;
}

static char *get_str(cxobj *xp, char *name)
{
	cxobj *obj;

	if (!xp || !name)
		return NULL;

	obj = xml_find(xp, name);
	if (obj)
		return xml_body(obj);

	return NULL;
}

static int sys_reload_services(void)
{
	return run("initctl -nbq touch sysklogd lldpd");
}

int ietf_sys_tr_begin(clicon_handle h, transaction_data td)
{
	return aug_load(aug);
}

int ietf_sys_tr_commit_hostname(cxobj *src, cxobj *tgt)
{
	const char *host, *new, *tmp;
	int err, i, nhosts;
	char **hosts, *old;

	if (src && xml_flag(src, XML_FLAG_DEL))
		new = "infix";	/* XXX: derive from global "options.h" */
	else if (tgt && xml_flag(tgt, XML_FLAG_ADD|XML_FLAG_CHANGE))
		new = xml_body(tgt);
	else
		return 0;

	aug_get(aug, "etc/hostname/hostname", &tmp);
	old = strdup(tmp);

	err = sethostname(new, strlen(new));
	err = err ? : aug_set(aug, "etc/hostname/hostname", new);

	nhosts = aug_match(aug, "etc/hosts/*/canonical", &hosts);
	for (i = 0; i < nhosts; i++) {
		aug_get(aug, hosts[i], &host);
		if (!strcmp(host, old))
			err = err ? : aug_set(aug, hosts[i], new);
		free(hosts[i]);
	}
	free(hosts);
	free(old);

	if (src)
		err = err ? : sys_reload_services();

	return err;
}

/*
 * On GLIBC systems the file /etc/timezone contains the name of the
 * current timezone, the file /etc/localtime is a copy or symlink to
 * the file /usr/share/zoneinfo/$(cat /etc/timezone)
 */
int ietf_sys_tr_commit_clock(cxobj *src, cxobj *tgt)
{
	const char *fn = "/etc/timezone";
	char *timezone;
	char cmd[512];
	cxobj *obj;
	FILE *fp;

	if (!tgt)
		return 0;

	obj = xml_find(tgt, "timezone-name");
	if (!obj)
		return 0;

	timezone = xml_body(obj);

	snprintf(cmd, sizeof(cmd), "cp /usr/share/zoneinfo/%s /etc/localtime", timezone);
	if (run(cmd)) {
		clicon_log(LOG_WARNING, "ietf-system: failed setting timezone %s", timezone);
		return -1;
	}

	fp = fopen(fn, "w");
	if (!fp) {
		clicon_log(LOG_WARNING, "ietf-system: failed updating %s: %s", fn, strerror(errno));
		return -1;
	}
	fprintf(fp, "%s\n", timezone);

	return fclose(fp);
}

int ietf_sys_tr_commit_ntp(cxobj *src, cxobj *tgt)
{
	const char *fn = "/etc/chrony.conf";
	cxobj *obj, *srv = NULL;
	int valid = 0;
	FILE *fp;

	if (!tgt) {
		if (src)
			goto disable;
		return 0;
	}

	fp = fopen(fn, "w");
	if (!fp) {
		clicon_log(LOG_WARNING, "ietf-system: failed updating %s: %s", fn, strerror(errno));
		return -1;
	}

	while ((srv = xml_child_each(tgt, srv, CX_ELMNT))) {
		char *type = "server";
		int server = 0;
		cxobj *tmp;

		if (strcmp(xml_name(srv), "server"))
			continue;

		obj = xml_find(srv, "association-type");
		if (obj)
			type = xml_body(obj);

		tmp = xml_find(srv, "udp");
		if (tmp) {
			obj = xml_find(tmp, "address");
			if (obj) {
				fprintf(fp, "%s %s", type, xml_body(obj));
				server++;
			}
			obj = xml_find(tmp, "port");
			if (server && obj)
				fprintf(fp, " port %s", xml_body(obj));
		}

		if (server) {
			if (is_true(srv, "iburst"))
				fprintf(fp, " iburst");
			if (is_true(srv, "prefer"))
				fprintf(fp, " prefer");
			fprintf(fp, "\n");
			valid++;
		}
	}

	fprintf(fp, "driftfile /var/lib/chrony/drift\n");
	fprintf(fp, "makestep 1.0 3\n");
	fprintf(fp, "maxupdateskew 100.0\n");
	fprintf(fp, "dumpdir /var/lib/chrony\n");
	fprintf(fp, "rtcfile /var/lib/chrony/rtc\n");
	fclose(fp);

	if (valid && is_true(tgt, "enabled")) {
		/*
		 * If chrony is alrady enabled we tell Finit it's been
		 * modified , so Finit restarts it, otherwise enable it.
		 */
		run("initctl -nbq touch chronyd");
		return run("initctl -nbq enable chronyd");
	}
disable:
	return run("initctl -nbq disable chronyd");
}

/*
 * dnsmasq is the resolver, it gets its nameservers from various
 * sources: static, dhcp, vpn, via the resolvconf wrapper.
 */
int ietf_sys_tr_commit_dns(cxobj *src, cxobj *tgt)
{
	const char *fn = "/etc/resolv.conf.head";
	cxobj *obj, *opt = NULL, *srv = NULL;
	int first = 1, valid = 0;
	FILE *fp;

	clicon_log(LOG_ERR, "ietf-system: dns src %p tgt %p", src, tgt);
	if (!tgt)
		goto disable;

	fp = fopen(fn, "w");
	if (!fp) {
		clicon_log(LOG_WARNING, "ietf-system: failed updating %s: %s", fn, strerror(errno));
		return -1;
	}

	obj = xml_find(tgt, "options");
	if (obj) {
		char *str;

		fprintf(fp, "options");
		str  = get_str(obj, "timeout");
		if (str)
			fprintf(fp, " timeout:%s", str);
		str = get_str(obj, "attempts");
		if (str)
			fprintf(fp, " attempts:%s ", str);
		fprintf(fp, "\n");
	}

	while ((opt = xml_child_each(tgt, opt, CX_ELMNT))) {
		if (strcmp(xml_name(opt), "search"))
			continue;

		if (first) {
			fprintf(fp, "search ");
			first = 0;
		}
		fprintf(fp, "%s ", xml_body(opt));
	}
	if (!first)
		fprintf(fp, "\n");

	while ((srv = xml_child_each(tgt, srv, CX_ELMNT))) {
		cxobj *tmp;

		if (strcmp(xml_name(srv), "server"))
			continue;

		tmp = xml_find(srv, "udp-and-tcp");
		if (tmp) {
			obj = xml_find(tmp, "address");
			if (obj) {
				fprintf(fp, "nameserver %s\n", xml_body(obj));
				valid++;
			}
		}
	}
	fclose(fp);
disable:
	if (!valid)
		remove(fn);

	if (run("initctl cond get hook/sys/up")) {
		clicon_log(LOG_ERR, "ietf-system: failed cond get hook/sys/up");
		return 0;
	}

	clicon_log(LOG_ERR, "ietf-status: calling resolvconf -u");
	return run("resolvconf -u");
}

int ietf_sys_tr_commit(clicon_handle h, transaction_data td)
{
	cxobj *src = transaction_src(td), *tgt = transaction_target(td);
	yang_stmt *yspec = clicon_dbspec_yang(h);
	int slen = 0, tlen = 0, err = -EINVAL;
	cxobj **ssys, **tsys;

	show_transaction("ietf-system", td, true);

	if (src && clixon_xml_find_instance_id(src, yspec, &ssys, &slen,
					       "/sys:system") < 0)
		goto err;

	if (tgt && clixon_xml_find_instance_id(tgt, yspec, &tsys, &tlen,
					       "/sys:system") < 0)
		goto err;

	err = ietf_sys_tr_commit_hostname(slen ? xml_find(ssys[0], "hostname") : NULL,
					  tlen ? xml_find(tsys[0], "hostname") : NULL);
	if (err)
		goto err;

	err = ietf_sys_tr_commit_clock(slen ? xml_find(ssys[0], "clock") : NULL,
				       tlen ? xml_find(tsys[0], "clock") : NULL);
	if (err)
		goto err;

	err = ietf_sys_tr_commit_ntp(slen ? xml_find(ssys[0], "ntp") : NULL,
				     tlen ? xml_find(tsys[0], "ntp") : NULL);
	if (err)
		goto err;

	err = ietf_sys_tr_commit_dns(slen ? xml_find(ssys[0], "dns-resolver") : NULL,
				     tlen ? xml_find(tsys[0], "dns-resolver") : NULL);
	if (err)
		goto err;
err:
	err = err ? : aug_save(aug);
	return err;
}

static char *strip_quotes(char *str)
{
	char *ptr;

	while (*str && (isspace(*str) || *str == '"'))
		str++;

	for (ptr = str + strlen(str); ptr > str; ptr--) {
		if (*ptr != '"')
			continue;

		*ptr = 0;
		break;
	}

	return str;
}

static void setvar(char *line, const char *nm, char **var)
{
	char *ptr;

	if (!strncmp(line, nm, strlen(nm)) && (ptr = strchr(line, '='))) {
		if (*var)
			free(*var);
		*var = strdup(strip_quotes(++ptr));
	}
}

static void os_init(void)
{
	struct utsname uts;
	char line[80];
	FILE *fp;

	if (!uname(&uts)) {
		os  = strdup(uts.sysname);
		ver = strdup(uts.release);
		rel = strdup(uts.release);
		sys = strdup(uts.machine);
	}

	fp = fopen("/etc/os-release", "r");
	if (!fp) {
		fp = fopen("/usr/lib/os-release", "r");
		if (!fp)
			return;
	}

	while (fgets(line, sizeof(line), fp)) {
		line[strlen(line) - 1] = 0; /* drop \n */
		setvar(line, "NAME", &os);
		setvar(line, "VERSION_ID", &ver);
		setvar(line, "BUILD_ID", &rel);
		setvar(line, "ARCHITECTURE", &sys);
	}
	fclose(fp);
}

static char *fmtime(time_t t, char *buf, size_t len)
{
        const char *isofmt = "%FT%T%z";
        struct tm tm;
        size_t i, n;

        localtime_r(&t, &tm);
        n = strftime(buf, len, isofmt, &tm);
        i = n - 5;
        if (buf[i] == '+' || buf[i] == '-') {
                buf[i + 6] = buf[i + 5];
                buf[i + 5] = buf[i + 4];
                buf[i + 4] = buf[i + 3];
                buf[i + 3] = ':';
        }

        return buf;
}

int ietf_sys_statedata(clicon_handle h, cvec *nsc, char *xpath, cxobj *xstate)
{
        struct sysinfo si;
        time_t now, boot;
        char buf[42];
	cbuf *cb;
	int rc;

	cb = cbuf_new();
	if (!cb) {
		clicon_err(OE_UNIX, errno, "ietf-system:cbuf_new");
		return -1;
	}

        tzset();
        sysinfo(&si);
        now = time(NULL);
        boot = now - si.uptime;

	cprintf(cb, "<system-state xmlns=\"%s\">", XMLNS);
	cprintf(cb, "  <platform>\n");
	cprintf(cb, "    <os-name>%s</os-name>\n", os);
	cprintf(cb, "    <os-version>%s</os-version>\n", ver);
	if (rel)
		cprintf(cb, "    <os-release>%s</os-release>\n", rel);
	cprintf(cb, "    <machine>%s</machine>\n", sys);
	cprintf(cb, "  </platform>\n");
	cprintf(cb, "  <clock>\n");
	cprintf(cb, "    <current-datetime>%s</current-datetime>\n", fmtime(now, buf, sizeof(buf)));
	cprintf(cb, "    <boot-datetime>%s</boot-datetime>\n", fmtime(boot, buf, sizeof(buf)));
	cprintf(cb, "  </clock>\n");
	cprintf(cb, "</system-state>");
	rc = clixon_xml_parse_string(cbuf_get(cb), YB_NONE, NULL, &xstate, NULL) < 0 ? -1 : 0;
	cbuf_free(cb);

	return rc;
}

static int set_datetime(cxobj *xe)
{
	const char *isofmt = "%FT%T%z";
        struct timeval tv;
        struct tm tm;
	char tz[24];
	int rc = -1;
	cxobj *obj;
	char *buf;
	size_t n;

	obj = xml_find(xe, "current-datetime");
	if (!obj)
		return 0;

	buf = strdup(xml_body(obj));
	if (!buf)
		return -1;

	n = strlen(buf);
	if (buf[n - 3] == ':' && (buf[n - 6] == '+' || buf[n - 6] == '-')) {
                buf[n - 3] = buf[n - 2];
                buf[n - 2] = buf[n - 1];
                buf[n - 1] = 0;
        } else
                isofmt = "%FT%TZ";

        memset(&tm, 0, sizeof(tm));
        if (!strptime(buf, isofmt, &tm)) {
                clicon_log(LOG_WARNING, "ietf-system:failed strptime: %s", strerror(errno));
		goto done;
	}

	snprintf(tz, sizeof(tz), "UTC%s%ld", tm.tm_gmtoff > 0 ? "+" : "", tm.tm_gmtoff / 3600);
	setenv("TZ", tz, 1);
//	clicon_log(LOG_ERR, "time %s gmtoff %ld TZ=%s", buf, tm.tm_gmtoff, tz);

        tv.tv_sec = mktime(&tm);
        tv.tv_usec = 0;
        if (settimeofday(&tv, NULL)) {
                clicon_log(LOG_WARNING, "ietf-system:failed settimeofday: %s", strerror(errno));
		goto done;
	}

	rc = 0;
done:
	unsetenv("TZ");
        free(buf);
	return rc;
}

static int do_rpc(clicon_handle h,            /* Clicon handle */
		  cxobj        *xe,           /* Request: <rpc><xn></rpc> */
		  cbuf         *cbret,        /* Reply eg <rpc-reply>... */
		  void         *arg,          /* client_entry */
		  void         *regarg)       /* Argument given at register */
{
	char *namespace;
	char *op;

	if ((namespace = xml_find_type_value(xe, NULL, "xmlns", CX_ATTR)) == NULL) {
		clicon_err(OE_XML, ENOENT, "No namespace given in RPC %s", xml_name(xe));
		return -1;
	}
	if (strcmp(namespace, XMLNS))
		return 0;	/* not for us */

	cprintf(cbret, "<rpc-reply xmlns=\"%s\"><ok/></rpc-reply>", NETCONF_BASE_NAMESPACE);

	op = xml_name(xe);
	if (!strcmp(op, "set-current-datetime"))
		return set_datetime(xe);
	if (!strcmp(op, "system-shutdown"))
		return run("poweroff");
	if (!strcmp(op, "system-restart"))
		return run("reboot");
	return 0;
}

static clixon_plugin_api ietf_system_api = {
	.ca_name = "ietf-system",
	.ca_init = clixon_plugin_init,

	.ca_trans_begin = ietf_sys_tr_begin,
	.ca_trans_commit = ietf_sys_tr_commit,

	.ca_statedata = ietf_sys_statedata,
};

static void register_rpc(clicon_handle h, const char *rpc)
{
	int rc;

	rc = rpc_callback_register(h, do_rpc, NULL, XMLNS, rpc);
	if (rc < 0)
		clicon_err(OE_PLUGIN, EINVAL, "ietf-system: failed registering RPC %s", rpc);
}

clixon_plugin_api *clixon_plugin_init(clicon_handle h)
{
	os_init();

	aug = aug_init(NULL, "", 0);
	if (!aug ||
	    aug_load_file(aug, "/etc/hostname") ||
	    aug_load_file(aug, "/etc/hosts")) {
		clicon_err(OE_UNIX, EINVAL,
			   "ietf-system: Augeas initialization failed");
		return NULL;
	}

	register_rpc(h, "set-current-datetime");
	register_rpc(h, "system-restart");
	register_rpc(h, "system-shutdown");

	return &ietf_system_api;
}
