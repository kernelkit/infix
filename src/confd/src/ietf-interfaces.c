/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"
#include "srx_module.h"
#include "srx_val.h"

static const char *ipfeat[] = {
	"ipv4-non-contiguous-netmasks",
	NULL
};

static const struct srx_module_requirement ietf_if_reqs[] = {
	{ .dir = YANG_PATH_, .name = "ietf-interfaces", .rev = "2018-02-20" },
	{ .dir = YANG_PATH_, .name = "iana-if-type", .rev = "2017-01-19" },
	{ .dir = YANG_PATH_, .name = "ietf-ip", .rev = "2018-02-22", .features = ipfeat },

	{ NULL }
};

static int inet_mask2len(char *netmask)
{
	struct in_addr ina;
	int len = 0;

	if (inet_pton(AF_INET, netmask, &ina) != 1)
		return 0;

	while (ina.s_addr) {
		ina.s_addr >>= 1;
		len++;
	}

	return len;
}

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
		if (ptr)
			writesf(ptr, "/sys/class/net/%s/ifalias", ifname);
		free(ptr);

		writedf(srx_enabled(session, "%s/ietf-ip:ipv4/forwarding", xpath),
			"/proc/sys/net/ipv4/conf/%s/forwarding", ifname);

		systemf("ip addr flush dev %s", ifname);

		snprintf(path, sizeof(path), "%s/ietf-ip:ipv4/address", xpath);
		rc = sr_get_items(session, path, 0, 0, &addr, &addrcnt);
		for (size_t j = 0; j < addrcnt; j++) {
			char *address, *netmask;
			int plen = 0;

			address = srx_get_str(session, "%s/ip", addr[j].xpath);
			SRX_GET_UINT8(session, plen, "%s/prefix-length", addr[j].xpath);
			if (!plen) {
				netmask = srx_get_str(session, "%s/netmask", addr[j].xpath);
				ERROR("read netmask instead: %s", netmask ?: "<NIL>");
				if (netmask) {
					plen = inet_mask2len(netmask);
					free(netmask);
				}
			}
			if (plen > 0) {
				char *add = "add";

				if (!srx_enabled(session, "%s/enabled", addr[j].xpath))
					add = "del";

				ERROR("Preparing to %s addess %s", add, address);
				systemf("ip addr add %s/%d dev %s", address, plen, ifname);
			}
			free(address);
		}

		systemf("ip link set %s %s", ifname, srx_enabled(session, "%s/enabled", xpath) ? "up" : "down");
		free(ifname);
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
