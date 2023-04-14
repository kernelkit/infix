/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"
#include "srx_module.h"
#include "srx_val.h"

const struct srx_module_requirement ietf_if_reqs[] = {
	{ .dir = YANG_PATH_, .name = "ietf-interfaces", .rev = "2018-02-20" },
	{ .dir = YANG_PATH_, .name = "iana-if-type", .rev = "2017-01-19" },
	{ .dir = YANG_PATH_, .name = "ietf-ip", .rev = "2018-02-22" },

	{ NULL }
};

static int ifchange(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		    const char *xpath, sr_event_t event, unsigned request_id, void *priv)
{
	sr_val_t *val;
	size_t cnt;
	int rc;

	if (event != SR_EV_DONE)
		return SR_ERR_OK;

	rc = sr_get_items(session, "/ietf-interfaces:interfaces/interface", 0, 0, &val, &cnt);
	if (rc)
		goto fail;

	for (size_t i = 0; i < cnt; i++) {
		const char *xpath = val[i].xpath;
		char path[strlen(xpath) + 64];
		sr_val_t *addr;
		size_t addrcnt;
		char *ifname;
		char *ptr;

		ifname = srx_get_str(session, "%s/name", xpath);
		ptr = srx_get_str(session, "%s/description", xpath);
		if (ptr) {
			FILE *fp;

			fp = fopenf("w", "/sys/class/net/%s/ifalias", ifname);
			if (fp) {
				fprintf(fp, "%s\n", ptr);
				fclose(fp);
			}
			free(ptr);
		}

		systemf("ip addr flush dev %s", ifname);

		snprintf(path, sizeof(path), "%s/ietf-ip:ipv4/address", xpath);
		rc = sr_get_items(session, path, 0, 0, &addr, &addrcnt);
		for (size_t j = 0; j < addrcnt; j++) {
			char *address;
			char *plen;

			address = srx_get_str(session, "%s/ip", addr[j].xpath);
			plen = srx_get_str(session, "%s/prefix-length", addr[j].xpath);
			systemf("ip addr add %s/%s dev %s", address, plen, ifname);
			free(address);
			free(plen);
		}

		systemf("ip link set %s %s", ifname, srx_enabled(session, "%s/enabled", xpath) ? "up" : "down");
	}
	sr_free_values(val, cnt);

	rc = SR_ERR_OK;
fail:
	return rc;
}

int ietf_interfaces_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, ietf_if_reqs);
	if (rc)
		goto err;

	REGISTER_CHANGE(confd->session, "ietf-interfaces", "/ietf-interfaces:interfaces", 0, ifchange, confd, &confd->sub);

	return SR_ERR_OK;
err:
	ERROR("init failed: %s", sr_strerror(rc));
	sr_unsubscribe(confd->sub);

	return rc;
}
