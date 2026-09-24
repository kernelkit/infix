/* SPDX-License-Identifier: BSD-3-Clause */

#include <ctype.h>

#include "core.h"

#define XPATH_SNMP_     "/ietf-snmp:snmp"
#define XPATH_SYSTEM_   "/ietf-system:system"

#define SNMP_DIR        "/etc/snmp"
#define SNMP_CONF       SNMP_DIR "/snmpd.conf"
#define SNMP_CONF_NEXT  SNMP_CONF "+"

#define LLDPD_DEFAULT   "/etc/default/lldpd"

/*
 * KernelKit PEN 61046, products arc, generic Infix.  Per-product arcs
 * are handed out from .1.1.2 and up when someone needs to tell two
 * Infix devices apart from the NMS.
 */
#define SNMP_SYSOBJECTID ".1.3.6.1.4.1.61046.1.1.1"

/* RFC 3411 caps the engine ID at 32 octets: "0x" + 64 hex + NUL */
#define MAX_ENGINE_ID 67

/* The single read-only MIB view every community is granted. */
#define SNMP_VIEW "infix"

/* Default when /snmp/engine/listen names no port. */
#define SNMP_PORT "161"

/*
 * infix-snmp restricts the leaves we own so a line break cannot reach
 * here, but /system/contact and /system/location belong to ietf-system
 * and are free text.  Drop anything that would not sit on one line of
 * snmpd.conf rather than let it add a directive.
 */
static int safe(const char *str)
{
	if (!str)
		return 0;

	for (; *str; str++) {
		if (!isprint((unsigned char)*str))
			return 0;
	}

	return 1;
}

static int is_v6(const char *addr)
{
	return strchr(addr, ':') != NULL;
}

/*
 * YANG gives the engine ID as colon-separated hex, net-snmp reads
 * anything without a 0x prefix as a literal ASCII string.
 */
static char *engine_id(const char *val, char *buf, size_t len)
{
	size_t pos = 0;

	if (len < 3)
		return NULL;

	buf[pos++] = '0';
	buf[pos++] = 'x';
	for (; *val; val++) {
		if (*val == ':')
			continue;
		if (!isxdigit((unsigned char)*val) || pos + 1 >= len)
			return NULL;
		buf[pos++] = *val;
	}
	buf[pos] = 0;

	return pos > 2 ? buf : NULL;
}

/*
 * "0.0.0.0" -> udp:0.0.0.0:161, "::" -> udp6:[::]:161.  With no listen
 * entries net-snmp falls back to every address on port 161 anyway, so
 * say so explicitly rather than leave the file silent about it.
 */
static void agentaddress(FILE *fp, struct lyd_node *engine)
{
	struct lyd_node *listen;
	int num = 0;

	LYX_LIST_FOR_EACH(lyd_child(engine), listen, "listen") {
		struct lyd_node *udp = lydx_get_child(listen, "udp");
		const char *addr, *port;

		if (!udp)
			continue;

		addr = lydx_get_cattr(udp, "ip");
		if (!addr)
			continue;
		port = lydx_get_cattr(udp, "port") ?: SNMP_PORT;

		fputs(num++ ? "," : "agentaddress ", fp);
		if (is_v6(addr))
			fprintf(fp, "udp6:[%s]:%s", addr, port);
		else
			fprintf(fp, "udp:%s:%s", addr, port);
	}

	if (num)
		fputc('\n', fp);
	else
		fprintf(fp, "agentaddress udp:0.0.0.0:%s,udp6:[::]:%s\n",
			SNMP_PORT, SNMP_PORT);
}

/* Does any target carry this tag? */
static int tag_in_use(struct lyd_node *snmp, const char *tag)
{
	struct lyd_node *target, *node;

	LYX_LIST_FOR_EACH(lyd_child(snmp), target, "target") {
		LYX_LIST_FOR_EACH(lyd_child(target), node, "tag") {
			if (!strcmp(lyd_get_value(node), tag))
				return 1;
		}
	}

	return 0;
}

/*
 * net-snmp wants the source on the com2sec line itself, so the target
 * list is walked once per community.
 */
static void com2sec_by_tag(FILE *fp, struct lyd_node *snmp, const char *tag,
			   const char *secname, const char *community)
{
	struct lyd_node *target;

	LYX_LIST_FOR_EACH(lyd_child(snmp), target, "target") {
		struct lyd_node *udp, *node;
		const char *addr, *plen;
		int match = 0;

		LYX_LIST_FOR_EACH(lyd_child(target), node, "tag") {
			if (!strcmp(lyd_get_value(node), tag)) {
				match = 1;
				break;
			}
		}
		if (!match)
			continue;

		udp = lydx_get_child(target, "udp");
		if (!udp)
			continue;

		addr = lydx_get_cattr(udp, "ip");
		if (!addr)
			continue;
		plen = lydx_get_cattr(udp, "prefix-length");

		fprintf(fp, "%s %s %s%s%s %s\n", is_v6(addr) ? "com2sec6" : "com2sec",
			secname, addr, plen ? "/" : "", plen ?: "", community);
	}
}

/*
 * The community string to advertise, or NULL if the entry cannot be
 * expressed.  RFC 7407 says an unset name falls back to the security
 * name; binary-name carries octets that snmpd.conf has no syntax for.
 */
static const char *community_name(struct lyd_node *community)
{
	const char *name = lydx_get_cattr(community, "text-name");

	if (name)
		return safe(name) ? name : NULL;
	if (lydx_get_cattr(community, "binary-name"))
		return NULL;

	name = lydx_get_cattr(community, "security-name");

	return safe(name) ? name : NULL;
}

/*
 * The security name a usable community maps to, NULL if we skip it.
 * Silent: communities() says why, and access_control() asks again for
 * every entry, so logging here would repeat it once per community.
 */
static const char *community_secname(struct lyd_node *snmp, struct lyd_node *community)
{
	const char *secname = lydx_get_cattr(community, "security-name");
	const char *tag;

	if (!safe(secname) || !community_name(community))
		return NULL;

	tag = lydx_get_cattr(community, "target-tag");
	if (tag && !tag_in_use(snmp, tag))
		return NULL;

	return secname;
}

static void communities(FILE *fp, struct lyd_node *snmp)
{
	struct lyd_node *community;

	LYX_LIST_FOR_EACH(lyd_child(snmp), community, "community") {
		const char *index = lydx_get_cattr(community, "index");
		const char *secname, *name, *tag;

		secname = lydx_get_cattr(community, "security-name");
		if (!safe(secname))
			continue;

		name = community_name(community);
		if (!name) {
			ERROR("SNMP community %s: binary-name is not supported", index);
			continue;
		}

		tag = lydx_get_cattr(community, "target-tag");
		if (!tag) {
			fprintf(fp, "com2sec %s default %s\n", secname, name);
			continue;
		}

		/*
		 * A target-tag is the operator asking for a source
		 * restriction.  If no target carries it, drop the
		 * community rather than fall back to any source, which
		 * would silently widen access.
		 */
		if (!tag_in_use(snmp, tag)) {
			ERROR("SNMP community %s: no target tagged %s, skipping", index, tag);
			continue;
		}

		com2sec_by_tag(fp, snmp, tag, secname, name);
	}
}

/*
 * VACM is deviated away: there is one view, it covers everything, and
 * nothing may write.  So rather than make the operator spell out a
 * group, a view and an access entry per community, confd writes the
 * same plumbing net-snmp's own rocommunity shorthand would, keeping
 * the operator's security name as the group name.
 */
static void access_control(FILE *fp, struct lyd_node *snmp, struct lyd_node *engine)
{
	struct lyd_node *version, *community, *seen;
	int v1, v2c, num = 0;

	/*
	 * RFC 7407 gives /snmp/engine/version no default.  Unset means
	 * the operator has no opinion, so serve both the versions we
	 * support, the same way net-snmp's rocommunity shorthand does.
	 */
	version = lydx_get_child(engine, "version");
	v1  = version && lydx_get_child(version, "v1");
	v2c = version && lydx_get_child(version, "v2c");
	if (!v1 && !v2c)
		v1 = v2c = 1;

	LYX_LIST_FOR_EACH(lyd_child(snmp), community, "community") {
		const char *secname = community_secname(snmp, community);
		int dup = 0;

		if (!secname)
			continue;

		/* Several communities may share a security name. */
		LYX_LIST_FOR_EACH(lyd_child(snmp), seen, "community") {
			const char *other;

			if (seen == community)
				break;
			other = community_secname(snmp, seen);
			if (other && !strcmp(other, secname)) {
				dup = 1;
				break;
			}
		}
		if (dup)
			continue;

		if (v1)
			fprintf(fp, "group %s v1 %s\n", secname, secname);
		if (v2c)
			fprintf(fp, "group %s v2c %s\n", secname, secname);
		fprintf(fp, "access %s \"\" any noauth exact %s none none\n",
			secname, SNMP_VIEW);
		num++;
	}

	if (num)
		fprintf(fp, "view %s included .1\n", SNMP_VIEW);
}

static int generate(struct lyd_node *config, struct lyd_node *snmp)
{
	struct lyd_node *engine, *system;
	const char *str;
	FILE *fp;

	engine = lydx_get_child(snmp, "engine");
	if (!engine)
		return SR_ERR_OK;

	fp = fopenp(SNMP_CONF_NEXT, 0640, NULL);
	if (!fp) {
		ERRNO("failed creating %s", SNMP_CONF_NEXT);
		return SR_ERR_SYS;
	}

	fprintf(fp, "# Generated by confd\n");
	agentaddress(fp, engine);

	fprintf(fp, "sysObjectID %s\n", SNMP_SYSOBJECTID);

	system = lydx_get_xpathf(config, XPATH_SYSTEM_);
	if (system) {
		str = lydx_get_cattr(system, "contact");
		if (str && safe(str))
			fprintf(fp, "syscontact %s\n", str);
		str = lydx_get_cattr(system, "location");
		if (str && safe(str))
			fprintf(fp, "syslocation %s\n", str);
	}

	str = lydx_get_cattr(engine, "engine-id");
	if (str) {
		char buf[MAX_ENGINE_ID];

		if (engine_id(str, buf, sizeof(buf)))
			fprintf(fp, "exactEngineID %s\n", buf);
		else
			ERROR("SNMP: invalid engine-id %s, using the default", str);
	}

	/* lldpd attaches over AgentX to serve LLDP-MIB. */
	fprintf(fp, "master agentx\n");

	/* UCD-SNMP-MIB's dskTable stays empty unless the agent is told which
	 * filesystems to account for. */
	fprintf(fp, "includeAllDisks 10%%\n");

	fputc('\n', fp);
	communities(fp, snmp);
	fputc('\n', fp);
	access_control(fp, snmp, engine);

	fclose(fp);

	return SR_ERR_OK;
}

/*
 * lldpd only registers LLDP-MIB with the master agent when started with
 * -x, so it has to be restarted when the agent comes or goes.
 */
static void lldpd_agentx(int ena)
{
	const char *args = ena ? "-x" : "";
	char *cur;
	FILE *fp;

	cur = fgetkey(LLDPD_DEFAULT, "LLDPD_ARGS");
	if (cur && !strcmp(cur, args))
		return;

	fp = fopen(LLDPD_DEFAULT, "w");
	if (!fp) {
		ERRNO("failed updating %s", LLDPD_DEFAULT);
		return;
	}

	fprintf(fp, "# Generated by confd\n");
	fprintf(fp, "LLDPD_ARGS=\"%s\"\n", args);
	fclose(fp);

	finit_reload("lldpd");
}

int snmp_change(sr_session_ctx_t *session, struct lyd_node *config, struct lyd_node *diff,
		sr_event_t event, struct confd *confd)
{
	struct lyd_node *snmp, *engine;
	int ena, rc;

	/*
	 * Hostname is here because sysName is the kernel hostname snmpd
	 * read when it started, so a change only takes effect when the
	 * agent is restarted below.
	 */
	if (diff && !lydx_get_xpathf(diff, XPATH_SNMP_) &&
	    !lydx_get_xpathf(diff, XPATH_SYSTEM_ "/contact") &&
	    !lydx_get_xpathf(diff, XPATH_SYSTEM_ "/location") &&
	    !lydx_get_xpathf(diff, XPATH_SYSTEM_ "/hostname"))
		return SR_ERR_OK;

	switch (event) {
	case SR_EV_CHANGE:
		break;

	case SR_EV_ABORT:
		(void)remove(SNMP_CONF_NEXT);
		return SR_ERR_OK;

	case SR_EV_DONE:
		snmp = lydx_get_xpathf(config, XPATH_SNMP_);
		engine = snmp ? lydx_get_child(snmp, "engine") : NULL;
		ena = engine && lydx_is_enabled(engine, "enabled");

		(void)remove(SNMP_CONF);
		(void)rename(SNMP_CONF_NEXT, SNMP_CONF);

		ena ? finit_enable("snmpd") : finit_disable("snmpd");
		if (ena)
			finit_reload("snmpd");
		lldpd_agentx(ena);

		return SR_ERR_OK;

	default:
		return SR_ERR_OK;
	}

	snmp = lydx_get_xpathf(config, XPATH_SNMP_);
	if (!snmp)
		return SR_ERR_OK;

	mkpath(SNMP_DIR, 0755);
	rc = generate(config, snmp);
	if (rc)
		(void)remove(SNMP_CONF_NEXT);

	return rc;
}
