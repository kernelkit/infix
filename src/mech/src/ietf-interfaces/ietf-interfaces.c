/* SPDX-License-Identifier: Apache-2.0 */

#include <ctype.h>
#include <errno.h>
#include <jansson.h>
#include <stdio.h>
#include <syslog.h>
#include <unistd.h>

#include "common.h"

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

int ietf_if_tr_begin(clicon_handle h, transaction_data td)
{
	return 0;
}

void sysfs_net_write(char *ifname, char *fn, char *data)
{
	char filename[128];
	FILE *fp;

	snprintf(filename, sizeof(filename), "/sys/class/net/%s/%s", ifname, fn);
	fp = fopen(filename, "w");
	if (!fp) {
		clicon_log(LOG_WARNING, "ietf-interfaces: failed writing '%s' to %s: %s",
			   data, filename, strerror(errno));
		return;
	}

	fprintf(fp, "%s\n", data);
	fclose(fp);
}

int ietf_if_tr_commit_interface(cxobj *src, cxobj *tgt)
{
	const char *fmt = "/etc/network/interfaces.d/%s.conf";
	cxobj *obj, *iface = NULL;

	if (!tgt) {
		clicon_log(LOG_DEBUG, "ietf-interfaces: no tgt, removing all network settings!");
		return system("rm -f /etc/network/interfaces.d/*");
	}

	while ((iface = xml_child_each(tgt, iface, CX_ELMNT))) {
		char *ifname, *addr, *len, *desc = NULL;
		char cmd[128], fn[60];
		cxobj *ip, *address;
		FILE *fp;

		if (strcmp(xml_name(iface), "interface")) {
			clicon_log(LOG_NOTICE, "Not an interface ...");
			continue;
		}

		obj = xml_find(iface, "name");
		if (!obj)
			continue;

		ifname = xml_body(obj);
		snprintf(fn, sizeof(fn), fmt, ifname);
		if (!is_true(iface, "enabled")) {
		delete:
			snprintf(cmd, sizeof(cmd), "rm -f %s", fn);
			system(cmd);
			continue;
		}

		obj = xml_find(iface, "description");
		if (obj)
			desc = xml_body(obj);

		ip = xml_find(iface, "ipv4");
		if (!ip || !is_true(ip, "enabled"))
			goto delete;

		/* XXX: iterate over address, may be more than one */
		address = xml_find(ip, "address");
		if (!address)
			goto delete;

		obj = xml_find(address, "ip");
		if (!obj)
			goto delete;
		addr = xml_body(obj);
		
		obj = xml_find(address, "prefix-length");
		if (!obj)
			goto delete;
		len = xml_body(obj);

		fp = fopen(fn, "w");
		if (!fp) {
			clicon_log(LOG_WARNING, "ietf-interfaces: failed creating %s: %s", fn, strerror(errno));
			return -1;
		}
		fprintf(fp, "auto %s\n", ifname);
		fprintf(fp, "iface %s inet static\n"
			"	address %s/%s\n", ifname, addr, len);
		fclose(fp);

		sysfs_net_write(ifname, "ifalias", desc ?: "");
	}

	return 0;
}

int ietf_if_tr_commit(clicon_handle h, transaction_data td)
{
	cxobj *src = transaction_src(td), *tgt = transaction_target(td);
	yang_stmt *yspec = clicon_dbspec_yang(h);
	int slen = 0, tlen = 0, err = -EINVAL;
	cxobj **ssys, **tsys;

	show_transaction("ietf-interfaces", td, true);

	if (src && clixon_xml_find_instance_id(src, yspec, &ssys, &slen,
					       "/if:interfaces") < 0)
		goto err;

	if (tgt && clixon_xml_find_instance_id(tgt, yspec, &tsys, &tlen,
					       "/if:interfaces") < 0)
		goto err;

	system("ifdown -a");
	err = ietf_if_tr_commit_interface(slen ? ssys[0] : NULL, tlen ? tsys[0] : NULL);
	system("ifup -a");
	if (err)
		goto err;
err:
	return err;
}

/* Verify obj is a string and then lowercase print it in fmt */
void cprint_xml_string(cbuf *cb, const char *fmt, json_t *obj, const char *key)
{
	json_t *val;
	char *str;

	val = json_object_get(obj, key);
	if (!val || !json_is_string(val))
		return;

	str = strdup(json_string_value(val));
	if (!str)
		return;

	for (size_t i = 0; i < strlen(str); i++)
		str[i] = tolower(str[i]);
	cprintf(cb, fmt, str);
	free(str);
}

void cprint_ifdata(cbuf *cb, char *ifname)
{
	json_t *root = NULL, *obj;
	json_error_t error;
	FILE *fp = NULL;
	char cmd[128];
	size_t len;
	char *buf;

	buf = malloc(BUFSIZ);
	if (!buf)
		goto err;

	snprintf(cmd, sizeof(cmd), "ip -j -p link show %s", ifname);
	fp = popen(cmd, "r");
	if (!fp)
		goto err;

	len = fread(buf, 1, 4096, fp);
	if (len == 0 || ferror(fp))
		goto err;

	root = json_loadb(buf, len, 0, &error);
	if (!root)
		goto err;
	if (!json_is_array(root)) {
		if (!json_is_object(root))
			goto err;
		obj = root;
	} else
		obj = json_array_get(root, 0);

	cprintf(cb,
		"<interface xmlns:ex=\"urn:example:clixon\">\n"
		"  <name>%s</name>\n"
		"  <type>ex:eth</type>\n", ifname);
	cprint_xml_string(cb, "  <phys-address>%s</phys-address>\n", obj, "address");
	cprint_xml_string(cb, "  <oper-status>%s</oper-status>\n", obj, "operstate");
	cprintf(cb, "</interface>");
err:
	if (root)
		json_decref(root);
	if (fp)
		pclose(fp);
	if (buf)
		free(buf);
}

int ietf_if_statedata(clicon_handle h, cvec *nsc, char *xpath, cxobj *xstate)
{
	cxobj    **xvec = NULL;
	size_t     xlen = 0;
	cbuf      *cb = NULL;
	cxobj     *xt = NULL;
	cvec      *nsc1 = NULL;
//	yang_stmt *yspec = NULL;
	int        rc = -1;

	if ((cb = cbuf_new()) == NULL) {
		clicon_err(OE_UNIX, errno, "cbuf_new");
		goto done;
	}

//	yspec = clicon_dbspec_yang(h);
	if ((nsc1 = xml_nsctx_init(NULL, "urn:ietf:params:xml:ns:yang:ietf-interfaces")) == NULL)
		goto done;
	if (xmldb_get0(h, "running", YB_MODULE, nsc1, "/interfaces/interface/name", 1, 0, &xt, NULL, NULL) < 0)
		goto done;
	if (xpath_vec(xt, nsc1, "/interfaces/interface/name", &xvec, &xlen) < 0)
		goto done;
	if (xlen) {
		cprintf(cb, "<interfaces xmlns=\"urn:ietf:params:xml:ns:yang:ietf-interfaces\">");
		for (size_t i = 0; i < xlen; i++)
			cprint_ifdata(cb, xml_body(xvec[i]));
		cprintf(cb, "</interfaces>");
		if (clixon_xml_parse_string(cbuf_get(cb), YB_NONE, NULL, &xstate, NULL) < 0)
			goto done;
	}
	rc = 0;
done:
	if (nsc1)
		xml_nsctx_free(nsc1);
	if (xt)
		xml_free(xt);
	if (cb)
		cbuf_free(cb);
	if (xvec)
		free(xvec);
	return rc;
}

static clixon_plugin_api ietf_interfaces_api = {
	.ca_name = "ietf-interfaces",
	.ca_init = clixon_plugin_init,

	.ca_trans_begin = ietf_if_tr_begin,
	.ca_trans_commit = ietf_if_tr_commit,

	.ca_statedata = ietf_if_statedata,
};

clixon_plugin_api *clixon_plugin_init(clicon_handle h)
{
	return &ietf_interfaces_api;
}
