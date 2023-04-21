/* SPDX-License-Identifier: BSD-3-Clause */

#include <net/if.h>

#include "core.h"
#include "srx_module.h"
#include "srx_val.h"

struct iface {
        TAILQ_ENTRY(iface) link;

        int      ifindex;
        char     ifname[IFNAMSIZ];
	char     hwaddr[18];
        short    flags;
	uint64_t speed;
	short    vid;
	short    pvid;
	char     upper[IFNAMSIZ];
};

static const char *iffeat[] = {
	"if-mib",
	NULL
};

static const char *ipfeat[] = {
	"ipv4-non-contiguous-netmasks",
	NULL
};

static const struct srx_module_requirement ietf_if_reqs[] = {
	{ .dir = YANG_PATH_, .name = "ietf-interfaces", .rev = "2018-02-20", .features = iffeat },
	{ .dir = YANG_PATH_, .name = "iana-if-type", .rev = "2023-01-26" },
	{ .dir = YANG_PATH_, .name = "ietf-ip", .rev = "2018-02-22", .features = ipfeat },

	{ NULL }
};

static TAILQ_HEAD(iflist, iface) iface_list = TAILQ_HEAD_INITIALIZER(iface_list);

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

static void ifprobe(void)
{
	struct if_nameindex *if_list, *i;

	if_list = if_nameindex();
	if (!if_list) {
		ERROR("failed if_nameindex(): %s", strerror(errno));
		return;
	}

	for (i = if_list; !(i->if_index == 0 && i->if_name == NULL); i++) {
		struct iface *iface;

		if (!i->if_name || i->if_index == 0)
			continue;

		iface = calloc(1, sizeof(struct iface));
		if (!iface) {
			ERROR("out of memory");
			return;
		}

		iface->ifindex = i->if_index;
		strlcpy(iface->ifname, i->if_name, sizeof(iface->ifname));

		/* XXX: add other data */
		TAILQ_INSERT_TAIL(&iface_list, iface, link);
	}

	if_freenameindex(if_list);
}

#define INTERFACE_XPATH "/ietf-interfaces:interfaces/interface[name='%s']"

static void ifpopul(sr_session_ctx_t *session)
{
	struct iface *iface;
	int rc = 0;

	TAILQ_FOREACH(iface, &iface_list, link) {
		char xpath[sizeof(INTERFACE_XPATH) + IFNAMSIZ + 42];
		sr_val_t val = { 0 };

		snprintf(xpath, sizeof(xpath), INTERFACE_XPATH "/if-index", iface->ifname);
		val.data.int32_val = iface->ifindex;
		val.type = SR_INT32_T;
		rc = sr_set_item(session, xpath, &val, 0);
		if (rc)
			ERROR("failed setting item %s", xpath);
		snprintf(xpath, sizeof(xpath), INTERFACE_XPATH "/admin-status", iface->ifname);
		val.data.enum_val = "up";
		val.type = SR_ENUM_T;
		rc = sr_set_item(session, xpath, &val, 0);
		if (rc)
			ERROR("failed setting item %s", xpath);

		if (!iface->hwaddr[0]) /* e.g., loopback */
			continue;

		snprintf(xpath, sizeof(xpath), INTERFACE_XPATH "/phys-address", iface->ifname);
		rc = sr_set_item_str(session, xpath, iface->hwaddr, NULL, 0);
		if (rc)
			ERROR("failed setting item %s", xpath);
	}

	rc = sr_apply_changes(session, 0);
	if (rc)
		ERROR("faled: %s", sr_strerror(rc));
}

static void ifinit(sr_session_ctx_t *session)
{
	struct iface *iface;
	int rc = 0;

	TAILQ_FOREACH(iface, &iface_list, link) {
		char xpath[sizeof(INTERFACE_XPATH) + IFNAMSIZ + 128];
		sr_val_t val = {};

		snprintf(xpath, sizeof(xpath), INTERFACE_XPATH "/type", iface->ifname);
		if (!strcmp("lo", iface->ifname))
			val.data.string_val = "iana-if-type:softwareLoopback";
		else if (!strncmp("eth", iface->ifname, 3))
			val.data.string_val = "iana-if-type:ethernetCsmacd";
		else
			continue;

		val.type = SR_IDENTITYREF_T;
		if ((rc = sr_set_item(session, xpath, &val, 0)))
			goto fail;

		if (strcmp(iface->ifname, "lo"))
			continue;

		snprintf(xpath, sizeof(xpath), INTERFACE_XPATH
			 "/ietf-ip:ipv4/address[ip='%s']/prefix-length", iface->ifname, "127.0.0.1");
		if ((rc = sr_set_item_str(session, xpath, "8", NULL, 0)))
			goto fail;

		snprintf(xpath, sizeof(xpath), INTERFACE_XPATH
			 "/ietf-ip:ipv6/address[ip='%s']/prefix-length", iface->ifname, "::1");
		rc = sr_set_item_str(session, xpath, "128", NULL, 0);
		if (rc) {
		fail:
			ERROR("failed setting item %s", xpath);
			continue;
		}
	}

	rc = sr_apply_changes(session, 0);
	if (rc)
		ERROR("faled: %s", sr_strerror(rc));
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
		int dad_xmit = 1;
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
		if (!srx_enabled(session, "%s/ietf-ip:ipv4/enabled", xpath))
			goto ipv6;

		snprintf(path, sizeof(path), "%s/ietf-ip:ipv4/address", xpath);
		rc = sr_get_items(session, path, 0, 0, &addr, &addrcnt);
		for (size_t j = 0; j < addrcnt; j++) {
			char *address, *netmask;
			int plen = 0;

			address = srx_get_str(session, "%s/ip", addr[j].xpath);
			SRX_GET_UINT8(session, plen, "%s/prefix-length", addr[j].xpath);
			if (!plen) {
				netmask = srx_get_str(session, "%s/netmask", addr[j].xpath);
				if (netmask) {
					plen = inet_mask2len(netmask);
					free(netmask);
				}
			}

			if (plen == 0)
				ERROR("%s: missing netmask or invalid prefix-length", address);
			else
				systemf("ip addr add %s/%d dev %s", address, plen, ifname);
			free(address);
		}
	ipv6:
		writedf(srx_enabled(session, "%s/ietf-ip:ipv6/forwarding", xpath),
			"/proc/sys/net/ipv6/conf/%s/forwarding", ifname);

		SRX_GET_UINT32(session, dad_xmit, "%s/ietf-ip:ipv6/dup-addr-detect-transmits", xpath);
		writedf(dad_xmit, "/proc/sys/net/ipv6/conf/%s/dad_transmits", ifname);

		if (!srx_enabled(session, "%s/ietf-ip:ipv6/enabled", xpath)) {
			writedf(1, "/proc/sys/net/ipv6/conf/%s/disable_ipv6", ifname);
			goto done;

		}
		writedf(0, "/proc/sys/net/ipv6/conf/%s/disable_ipv6", ifname);

		writedf(srx_enabled(session, "%s/ietf-ip:ipv6/autoconf/create-global-addresses", xpath),
			"/proc/sys/net/ipv6/conf/%s/autoconf", ifname);

		snprintf(path, sizeof(path), "%s/ietf-ip:ipv6/address", xpath);
		rc = sr_get_items(session, path, 0, 0, &addr, &addrcnt);
		for (size_t j = 0; j < addrcnt; j++) {
			char *address;
			int plen = 0;

			address = srx_get_str(session, "%s/ip", addr[j].xpath);
			SRX_GET_UINT8(session, plen, "%s/prefix-length", addr[j].xpath);
			if (plen == 0)
				ERROR("%s: missing netmask or invalid prefix-length", address);
			else
				systemf("ip addr add %s/%d dev %s", address, plen, ifname);
			free(address);
		}
	done:
		systemf("ip link set %s %s", ifname, srx_enabled(session, "%s/enabled", xpath) ? "up" : "down");
		free(ifname);
	}
	sr_free_values(val, cnt);

	rc = SR_ERR_OK;
fail:
	return rc;
}

static int ifoper(sr_session_ctx_t *session, uint32_t sub_id, const char *module,
		  const char *path, const char *request_path, uint32_t request_id,
		  struct lyd_node **parent, void *priv)
{
	char xpath[sizeof(INTERFACE_XPATH) + IFNAMSIZ + 42];
	const struct ly_ctx *ctx;
	struct iface *iface;
	int first = 1;
	int rc = 0;

	ctx = sr_acquire_context(sr_session_get_connection(session));

	TAILQ_FOREACH(iface, &iface_list, link) {
		snprintf(xpath, sizeof(xpath), INTERFACE_XPATH, iface->ifname);

		if ((rc = lydx_new_path(ctx, parent, &first, xpath, "if-index", "%d", iface->ifindex)))
			goto fail;

		if ((rc = lydx_new_path(ctx, parent, &first, xpath, "admin-status", "up")))
			goto fail;

		if ((rc = lydx_new_path(ctx, parent, &first, xpath, "oper-status", "up")))
			goto fail;

		if (rc) {
		fail:
			ERROR("Failed building data tree, libyang error %d", rc);
			sr_release_context(sr_session_get_connection(session));
			return SR_ERR_INTERNAL;
		}
	}

	sr_release_context(sr_session_get_connection(session));
	return SR_ERR_OK;
}

int ietf_interfaces_init(struct confd *confd)
{
	int rc;

	rc = srx_require_modules(confd->conn, ietf_if_reqs);
	if (rc)
		goto fail;

	ifprobe();

	REGISTER_CHANGE(confd->session, "ietf-interfaces", "/ietf-interfaces:interfaces", 0, ifchange, confd, &confd->sub);
	REGISTER_OPER(confd->session, "ietf-interfaces", "/ietf-interfaces:interfaces", ifoper, NULL, 0, &confd->sub);

	sr_session_switch_ds(confd->session, SR_DS_OPERATIONAL);
	ifpopul(confd->session);
	sr_session_switch_ds(confd->session, SR_DS_RUNNING);
	ifinit(confd->session);

	return SR_ERR_OK;
fail:
	ERROR("init failed: %s", sr_strerror(rc));
	return rc;
}
