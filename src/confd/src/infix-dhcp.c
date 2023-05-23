/* SPDX-License-Identifier: BSD-3-Clause */

#include <assert.h>
#include <ctype.h>
#include <pwd.h>
#include <sys/utsname.h>
#include <sys/sysinfo.h>
#include <sys/types.h>

#include "core.h"
#include "lyx.h"
#include "srx_module.h"
#include "srx_val.h"

static const struct srx_module_requirement infix_dhcp_reqs[] = {
	{ .dir = YANG_PATH_, .name = "infix-dhcp-client", .rev = "2023-05-22" },
	{ NULL }
};

static int is_enabled(struct lyd_node *parent, const char *name)
{
	const char *attr;

	attr = lydx_get_cattr(parent, name);
	if (!attr || !strcmp(attr, "true"))
		return 1;

	return 0;
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

	ERROR("Got DHCP client change");
	err = sr_get_data(session, "/dhcp-client//.", 0, 0, 0, &cfg);
	if (err)
		goto err_abandon;

	err = srx_get_diff(session, (struct lyd_node **)&diff);
	if (err)
		goto err_release_data;

	global = lydx_get_descendant(cfg->tree, "dhcp-client", NULL);
	ena = is_enabled(global, "enabled");

	cifs = lydx_get_descendant(cfg->tree, "dhcp-client", "client-if", NULL);
	difs = lydx_get_descendant(diff, "dhcp-client", "client-if", NULL);

	LYX_LIST_FOR_EACH(difs, dif, "client-if") {
		/* find the modified one, delete or recreate only that */
		LYX_LIST_FOR_EACH(cifs, cif, "client-if") {
			const char *ifname = lydx_get_cattr(dif, "if-name");
			FILE *fp;

			if (strcmp(ifname, lydx_get_cattr(cif, "if-name")))
				continue;

			if (!ena || !is_enabled(cif, "enabled")) {
				systemf("initctl delete dhcp-%s", ifname);
				continue;
			}

			fp = fopenf("w", "/etc/finit.d/available/dhcp-%s.conf", ifname);
			if (!fp) {
				ERROR("failed creating DHCP client service for %s: %s",
				      ifname, strerror(errno));
				continue;
			}

			fprintf(fp, "service name:dhcp :%s udhcpc -f -S -i %s -- DHCP client @%s",
				ifname, ifname, ifname);
			fclose(fp);
			systemf("initctl enable dhcp-%s", ifname);
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

	ERROR("Loaded DHCP model");
	REGISTER_CHANGE(confd->session, "infix-dhcp-client", "/infix-dhcp-client:dhcp-client",
			0, client_change, confd, &confd->sub);
	ERROR("Registered DHCP change callback");
	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
