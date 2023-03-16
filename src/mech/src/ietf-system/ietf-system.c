/* SPDX-License-Identifier: Apache-2.0 */

#include <augeas.h>
#include <errno.h>
#include <stdio.h>
#include <syslog.h>
#include <unistd.h>

#include "common.h"

static augeas *aug;

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

static int sys_reload_services(void)
{
	return system("initctl -nbq touch sysklogd lldpd");
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
	if (system(cmd)) {
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

	if (!tgt)
		return 0;

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
		system("initctl -nbq touch chronyd");
		return system("initctl -nbq enable chronyd");
	}

	return system("initctl -nbq disable chronyd");
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
err:
	err = err ? : aug_save(aug);
	return err;
}

static clixon_plugin_api ietf_system_api = {
	.ca_name = "ietf-system",
	.ca_init = clixon_plugin_init,

	.ca_trans_begin = ietf_sys_tr_begin,
	.ca_trans_commit = ietf_sys_tr_commit,
};

clixon_plugin_api *clixon_plugin_init(clicon_handle h)
{
	aug = aug_init(NULL, "", 0);
	if (!aug ||
	    aug_load_file(aug, "/etc/hostname") ||
	    aug_load_file(aug, "/etc/hosts")) {
		clicon_err(OE_UNIX, EINVAL,
			   "ietf-system: Augeas initialization failed");
		return NULL;
	}

	return &ietf_system_api;
}
