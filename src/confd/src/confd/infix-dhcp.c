/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <pwd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include "core.h"
#include "../lib/common.h"
#include "../lib/lyx.h"
#include "../lib/srx_module.h"
#include "srx_val.h"

static const struct srx_module_requirement infix_dhcp_reqs[] = {
	{ .dir = YANG_PATH_, .name = "infix-dhcp-client", .rev = "2023-05-22" },
	{ NULL }
};

static void add(const char *ifname, const char *client_id)
{
	char *args = NULL;
	FILE *fp;

	fp = fopenf("w", "/etc/finit.d/available/dhcp-%s.conf", ifname);
	if (!fp) {
		ERROR("failed creating DHCP client service for %s: %s",
		      ifname, strerror(errno));
		return;
	}

	if (client_id && client_id[0]) {
		args = alloca(strlen(client_id) + 12);
		if (args)
			sprintf(args, "-C -x 61:'\"%s\"'", client_id);
	}
	fprintf(fp, "service name:dhcp :%s udhcpc -f -S -R -i %s %s -- DHCP client @%s\n",
		ifname, ifname, args ?: "", ifname);
	fclose(fp);

	if (systemf("initctl -bfq enable dhcp-%s", ifname))
		ERROR("failed enabling DHCP client on %s", ifname);
}

static void del(const char *ifname)
{
	systemf("initctl -bfq delete dhcp-%s", ifname);
}

static int client_change(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
	const char *xpath, sr_event_t event, unsigned request_id, void *_confd)
{
	struct lyd_node *global, *diff, *cifs, *difs, *cif, *dif;
	sr_error_t err = 0;
	sr_data_t *cfg;
	int ena = 0;

	switch (event) {
	case SR_EV_DONE:
		break;
	case SR_EV_CHANGE:
	case SR_EV_ABORT:
	default:
		return SR_ERR_OK;
	}

	err = sr_get_data(session, "/infix-dhcp-client:dhcp-client//.", 0, 0, 0, &cfg);
	if (err) {
		ERROR("DHCP client fail 1");
		goto err_abandon;
	}

	err = srx_get_diff(session, &diff);
	if (err)
		goto err_release_data;

	global = lydx_get_descendant(cfg->tree, "dhcp-client", NULL);
	ena = lydx_is_enabled(global, "enabled");

	cifs = lydx_get_descendant(cfg->tree, "dhcp-client", "client-if", NULL);
	difs = lydx_get_descendant(diff, "dhcp-client", "client-if", NULL);

	/* find the modified one, delete or recreate only that */
	LYX_LIST_FOR_EACH(difs, dif, "client-if") {
		const char *ifname = lydx_get_cattr(dif, "if-name");

		if (lydx_get_op(dif) == LYDX_OP_DELETE) {
			del(ifname);
			continue;
		}

		LYX_LIST_FOR_EACH(cifs, cif, "client-if") {
			if (strcmp(ifname, lydx_get_cattr(cif, "if-name")))
				continue;

			if (!ena || !lydx_is_enabled(cif, "enabled"))
				del(ifname);
			else
				add(ifname, lydx_get_cattr(cif, "client-id"));
			break;
		}
	}

	lyd_free_tree(diff);
err_release_data:
	sr_release_data(cfg);
err_abandon:

	return err;
}


int infix_dhcp_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, infix_dhcp_reqs);
	if (rc)
		goto fail;

	REGISTER_CHANGE(confd->session, "infix-dhcp-client", "/infix-dhcp-client:dhcp-client",
			0, client_change, confd, &confd->sub);
	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
