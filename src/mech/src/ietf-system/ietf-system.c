/*
 * Copyright 2023  The KernelKit Authors <kernelkit@googlegroups.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include <augeas.h>
#include <errno.h>
#include <stdio.h>
#include <unistd.h>

#include <cligen/cligen.h>
#include <clixon/clixon.h>
#include <clixon/clixon_backend.h>

static augeas *aug;

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
		new = "infix";
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
		err = err ? : system("initctl touch sysklogd");

	return err;
}

int ietf_sys_tr_commit(clicon_handle h, transaction_data td)
{
	cxobj *src = transaction_src(td), *tgt = transaction_target(td);
	yang_stmt *yspec = clicon_dbspec_yang(h);
	int slen = 0, tlen = 0, err = -EINVAL;
	cxobj **ssys, **tsys;

	if (src && clixon_xml_find_instance_id(src, yspec, &ssys, &slen,
					       "/sys:system") < 0)
		goto err;

	if (tgt && clixon_xml_find_instance_id(tgt, yspec, &tsys, &tlen,
					       "/sys:system") < 0)
		goto err;

	err = ietf_sys_tr_commit_hostname(slen ? xml_find(ssys[0], "hostname") : NULL,
					  tlen ? xml_find(tsys[0], "hostname") : NULL);
	/* if (err) */
	/* 	goto err; */

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
