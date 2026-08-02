/* SPDX-License-Identifier: BSD-3-Clause */

/*
 * avahi.c - mDNS neighbor table for statd using libavahi-client + libev.
 *
 * Discovery flow:
 *   1. AvahiServiceTypeBrowser finds all service types on the local network.
 *   2. For each type, an AvahiServiceBrowser enumerates service instances.
 *   3. For each instance, a transient AvahiServiceResolver resolves hostname,
 *      address, port and TXT records.
 *   4. Resolved data is pushed to SR_DS_OPERATIONAL under
 *      /infix-services:mdns/neighbors via sr_set_item_str() + sr_apply_changes().
 *   5. On BROWSER_REMOVE, the corresponding DS subtree is deleted.
 *
 * The AvahiPoll vtable bridges avahi's event model to the main libev loop
 * (same thread — no locking required).
 */

#include <stdbool.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/queue.h>
#include <sys/time.h>
#include <time.h>

#include <ev.h>
#include <avahi-client/client.h>
#include <avahi-client/lookup.h>
#include <avahi-common/address.h>
#include <avahi-common/error.h>
#include <avahi-common/watch.h>
#include <sysrepo.h>

#include <srx/common.h>

#include "avahi.h"

/* Complete the opaque avahi types declared in avahi-common/watch.h */
struct AvahiWatch {
	ev_io              io;         /* MUST be first (cast from ev_io *) */
	AvahiWatchEvent    last_event;
	AvahiWatchCallback callback;
	void              *userdata;
	struct mdns_ctx  *ctx;
};

struct AvahiTimeout {
	ev_timer             timer;    /* MUST be first */
	AvahiTimeoutCallback callback;
	void                *userdata;
	struct mdns_ctx    *ctx;
};

/* --------------------------------------------------------------------------
 * libev-backed AvahiPoll vtable
 * -------------------------------------------------------------------------- */

static void watch_io_cb(struct ev_loop *loop, ev_io *w, int events)
{
	struct AvahiWatch *watch = (struct AvahiWatch *)w;
	AvahiWatchEvent av = 0;

	(void)loop;
	if (events & EV_READ)  av |= AVAHI_WATCH_IN;
	if (events & EV_WRITE) av |= AVAHI_WATCH_OUT;
	if (events & EV_ERROR) av |= AVAHI_WATCH_ERR;
	watch->last_event = av;
	watch->callback(watch, w->fd, av, watch->userdata);
}

static AvahiWatch *watch_new(const AvahiPoll *api, int fd, AvahiWatchEvent event,
			     AvahiWatchCallback callback, void *userdata)
{
	struct mdns_ctx *ctx = api->userdata;
	struct AvahiWatch *w;
	int ev_events = 0;

	w = calloc(1, sizeof(*w));
	if (!w)
		return NULL;

	w->callback = callback;
	w->userdata = userdata;
	w->ctx      = ctx;

	if (event & AVAHI_WATCH_IN)  ev_events |= EV_READ;
	if (event & AVAHI_WATCH_OUT) ev_events |= EV_WRITE;

	ev_io_init(&w->io, watch_io_cb, fd, ev_events);
	if (ev_events)
		ev_io_start(ctx->loop, &w->io);

	return w;
}

static void watch_update(AvahiWatch *w, AvahiWatchEvent event)
{
	int ev_events = 0;

	ev_io_stop(w->ctx->loop, &w->io);
	if (event & AVAHI_WATCH_IN)  ev_events |= EV_READ;
	if (event & AVAHI_WATCH_OUT) ev_events |= EV_WRITE;
	ev_io_set(&w->io, w->io.fd, ev_events);
	if (ev_events)
		ev_io_start(w->ctx->loop, &w->io);
}

static AvahiWatchEvent watch_get_events(AvahiWatch *w)
{
	return w->last_event;
}

static void watch_free(AvahiWatch *w)
{
	ev_io_stop(w->ctx->loop, &w->io);
	free(w);
}

static void timeout_cb(struct ev_loop *loop, ev_timer *t, int events)
{
	struct AvahiTimeout *timeout = (struct AvahiTimeout *)t;

	(void)loop;
	(void)events;
	timeout->callback(timeout, timeout->userdata);
}

static AvahiTimeout *timeout_new(const AvahiPoll *api, const struct timeval *tv,
				 AvahiTimeoutCallback callback, void *userdata)
{
	struct mdns_ctx *ctx = api->userdata;
	struct AvahiTimeout *t;

	t = calloc(1, sizeof(*t));
	if (!t)
		return NULL;

	t->callback = callback;
	t->userdata = userdata;
	t->ctx      = ctx;

	ev_timer_init(&t->timer, timeout_cb, 0.0, 0.0);

	if (tv) {
		struct timeval now;
		double delay;

		gettimeofday(&now, NULL);
		delay = (double)(tv->tv_sec  - now.tv_sec) +
			(double)(tv->tv_usec - now.tv_usec) / 1e6;
		if (delay < 0.0)
			delay = 0.0;
		ev_timer_set(&t->timer, delay, 0.0);
		ev_timer_start(ctx->loop, &t->timer);
	}
	/* NULL tv means disabled timer — do not start */

	return t;
}

static void timeout_update(AvahiTimeout *t, const struct timeval *tv)
{
	ev_timer_stop(t->ctx->loop, &t->timer);

	if (tv) {
		struct timeval now;
		double delay;

		gettimeofday(&now, NULL);
		delay = (double)(tv->tv_sec  - now.tv_sec) +
			(double)(tv->tv_usec - now.tv_usec) / 1e6;
		if (delay < 0.0)
			delay = 0.0;
		ev_timer_set(&t->timer, delay, 0.0);
		ev_timer_start(t->ctx->loop, &t->timer);
	}
}

static void timeout_free(AvahiTimeout *t)
{
	ev_timer_stop(t->ctx->loop, &t->timer);
	free(t);
}

/* --------------------------------------------------------------------------
 * In-memory state helpers
 * -------------------------------------------------------------------------- */

static struct avahi_neighbor *find_neighbor(struct mdns_ctx *ctx, const char *hostname)
{
	struct avahi_neighbor *n;

	LIST_FOREACH(n, &ctx->neighbors, link) {
		if (!strcmp(n->hostname, hostname))
			return n;
	}
	return NULL;
}

static struct avahi_neighbor *get_neighbor(struct mdns_ctx *ctx, const char *hostname)
{
	struct avahi_neighbor *n = find_neighbor(ctx, hostname);

	if (n)
		return n;

	n = calloc(1, sizeof(*n));
	if (!n)
		return NULL;

	snprintf(n->hostname, sizeof(n->hostname), "%s", hostname);
	LIST_INIT(&n->addrs);
	LIST_INSERT_HEAD(&ctx->neighbors, n, link);

	return n;
}

static int has_addr(struct avahi_neighbor *n, const char *addr)
{
	struct avahi_addr *a;

	LIST_FOREACH(a, &n->addrs, link) {
		if (!strcmp(a->val, addr))
			return 1;
	}
	return 0;
}

static void add_addr(struct avahi_neighbor *n, const char *addr)
{
	struct avahi_addr *a = calloc(1, sizeof(*a));

	if (!a)
		return;
	snprintf(a->val, sizeof(a->val), "%s", addr);
	LIST_INSERT_HEAD(&n->addrs, a, link);
}

/*
 * Find service in flat list by 5-tuple (ifindex, proto, name, type, domain).
 */
static struct avahi_service *find_service(struct mdns_ctx *ctx,
					  int ifindex, AvahiProtocol proto,
					  const char *name, const char *type,
					  const char *domain)
{
	struct avahi_service *s;

	LIST_FOREACH(s, &ctx->services, link) {
		if (s->ifindex == ifindex && s->proto == proto &&
		    !strcmp(s->name, name) && !strcmp(s->type, type) &&
		    !strcmp(s->domain, domain))
			return s;
	}
	return NULL;
}

/*
 * Check whether any service in the flat list matches (hostname, name) —
 * used after removing one 5-tuple entry to decide if the DS entry should
 * be removed too (another interface may still have the same service).
 */
static int svc_ds_entry_exists(struct mdns_ctx *ctx, const char *hostname, const char *name)
{
	struct avahi_service *s;

	LIST_FOREACH(s, &ctx->services, link) {
		if (!strcmp(s->hostname, hostname) && !strcmp(s->name, name))
			return 1;
	}
	return 0;
}

static int neighbor_has_services(struct mdns_ctx *ctx, const char *hostname)
{
	struct avahi_service *s;

	LIST_FOREACH(s, &ctx->services, link) {
		if (!strcmp(s->hostname, hostname))
			return 1;
	}
	return 0;
}

static void free_txts(struct avahi_service *svc)
{
	struct avahi_txt *t;

	while (!LIST_EMPTY(&svc->txts)) {
		t = LIST_FIRST(&svc->txts);
		LIST_REMOVE(t, link);
		free(t);
	}
}

static void free_service(struct avahi_service *svc)
{
	free_txts(svc);
	LIST_REMOVE(svc, link);
	free(svc);
}

static void free_neighbor(struct avahi_neighbor *n)
{
	struct avahi_addr *a;

	while (!LIST_EMPTY(&n->addrs)) {
		a = LIST_FIRST(&n->addrs);
		LIST_REMOVE(a, link);
		free(a);
	}
	LIST_REMOVE(n, link);
	free(n);
}

static void free_all(struct mdns_ctx *ctx)
{
	struct avahi_service *s;
	struct avahi_neighbor *n;

	while (!LIST_EMPTY(&ctx->services)) {
		s = LIST_FIRST(&ctx->services);
		free_service(s);
	}
	while (!LIST_EMPTY(&ctx->neighbors)) {
		n = LIST_FIRST(&ctx->neighbors);
		free_neighbor(n);
	}
}

/* --------------------------------------------------------------------------
 * sysrepo push helpers
 * -------------------------------------------------------------------------- */

#define XPATH_BASE "/infix-services:mdns/neighbors"

static void format_timestamp(char *buf, size_t sz)
{
	struct tm tm;
	time_t now = time(NULL);

	gmtime_r(&now, &tm);
	strftime(buf, sz, "%Y-%m-%dT%H:%M:%S+00:00", &tm);
}

static int sr_setstr(sr_session_ctx_t *ses, const char *xpath, const char *val)
{
	int err = sr_set_item_str(ses, xpath, val, NULL, 0);

	if (err)
		ERROR("mdns: sr_set_item_str(%s): %s", xpath, sr_strerror(err));
	return err;
}

/*
 * Return an XPath string literal quoting val: single-quoted unless val
 * contains a single quote, in which case double quotes are used instead.
 * buf must be at least strlen(val)+3 bytes.
 */
static const char *xpath_str(char *buf, size_t sz, const char *val)
{
	if (strchr(val, '\''))
		snprintf(buf, sz, "\"%s\"", val);
	else
		snprintf(buf, sz, "'%s'", val);
	return buf;
}

/*
 * Push a resolver result to the operational DS.
 * new_addr is non-NULL only when a new address was just added in memory.
 */
static void ds_push_resolver(struct mdns_ctx *ctx, struct avahi_service *svc,
			     const char *new_addr)
{
	char qname[258]; /* quoted svc->name for safe XPath predicates */
	char xpath[640];
	char val[64];
	struct avahi_txt *t;
	char ts[32];
	int err;

	xpath_str(qname, sizeof(qname), svc->name);

	/* Create neighbor list instance (key embedded in predicate; sysrepo 4.x
	 * rejects editing list-key leaves directly — set the list entry instead) */
	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']", svc->hostname);
	err = sr_setstr(ctx->sr_ses, xpath, NULL);

	/* address (only if a new one was added) */
	if (new_addr) {
		snprintf(xpath, sizeof(xpath),
			 XPATH_BASE "/neighbor[hostname='%s']/address", svc->hostname);
		err = err ?: sr_setstr(ctx->sr_ses, xpath, new_addr);
	}

	/* last-seen */
	format_timestamp(ts, sizeof(ts));
	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']/last-seen", svc->hostname);
	err = err ?: sr_setstr(ctx->sr_ses, xpath, ts);

	/* Delete and recreate service entry so TXT records are always fresh */
	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']/service[name=%s]",
		 svc->hostname, qname);
	sr_delete_item(ctx->sr_ses, xpath, 0);

	/* Create service list instance (same pattern — key in predicate) */
	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']/service[name=%s]",
		 svc->hostname, qname);
	err = err ?: sr_setstr(ctx->sr_ses, xpath, NULL);

	/* service/type */
	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']/service[name=%s]/type",
		 svc->hostname, qname);
	err = err ?: sr_setstr(ctx->sr_ses, xpath, svc->type);

	/* service/port */
	snprintf(val, sizeof(val), "%u", (unsigned)svc->port);
	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']/service[name=%s]/port",
		 svc->hostname, qname);
	err = err ?: sr_setstr(ctx->sr_ses, xpath, val);

	/* service/txt (leaf-list) */
	LIST_FOREACH(t, &svc->txts, link) {
		snprintf(xpath, sizeof(xpath),
			 XPATH_BASE "/neighbor[hostname='%s']/service[name=%s]/txt",
			 svc->hostname, qname);
		err = err ?: sr_setstr(ctx->sr_ses, xpath, t->val);
	}

	if (err) {
		sr_discard_changes(ctx->sr_ses);
		return;
	}

	err = sr_apply_changes(ctx->sr_ses, 0);
	if (err)
		ERROR("mdns: sr_apply_changes: %s", sr_strerror(err));
}

static void ds_delete_service(struct mdns_ctx *ctx, const char *hostname, const char *name)
{
	char qname[258];
	char xpath[512];

	xpath_str(qname, sizeof(qname), name);
	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']/service[name=%s]",
		 hostname, qname);
	sr_delete_item(ctx->sr_ses, xpath, 0);
}

static void ds_delete_neighbor(struct mdns_ctx *ctx, const char *hostname)
{
	char xpath[512];

	snprintf(xpath, sizeof(xpath),
		 XPATH_BASE "/neighbor[hostname='%s']", hostname);
	sr_delete_item(ctx->sr_ses, xpath, 0);
}

static void ds_clear_all(struct mdns_ctx *ctx)
{
	sr_delete_item(ctx->sr_ses, XPATH_BASE, 0);
	sr_apply_changes(ctx->sr_ses, 0);
}

/* --------------------------------------------------------------------------
 * Avahi callbacks
 * -------------------------------------------------------------------------- */

static void resolver_cb(AvahiServiceResolver *r,
			AvahiIfIndex iface, AvahiProtocol proto,
			AvahiResolverEvent event,
			const char *name, const char *type, const char *domain,
			const char *hostname, const AvahiAddress *addr,
			uint16_t port, AvahiStringList *txtlist,
			AvahiLookupResultFlags flags,
			void *userdata)
{
	struct mdns_ctx *ctx = userdata;
	char addrstr[AVAHI_ADDRESS_STR_MAX] = "";
	struct avahi_neighbor *n;
	struct avahi_service *svc;
	const char *new_addr = NULL;
	AvahiStringList *s;
	struct avahi_txt *t;
	int is_loopback;

	(void)flags;

	if (event != AVAHI_RESOLVER_FOUND)
		goto done;

	if (addr)
		avahi_address_snprint(addrstr, sizeof(addrstr), addr);

	is_loopback = (!strcmp(addrstr, "127.0.0.1") ||
		       !strcmp(addrstr, "::1") ||
		       !strncmp(addrstr, "127.", 4));

	/* Find or create neighbor (tracks addresses) */
	n = get_neighbor(ctx, hostname);
	if (!n) {
		ERROR("mdns: out of memory for neighbor '%s'", hostname);
		goto done;
	}

	/* Add address only if new and not loopback */
	if (!is_loopback && addrstr[0] && !has_addr(n, addrstr)) {
		add_addr(n, addrstr);
		new_addr = addrstr;
	}

	/* Find or create service entry in flat list */
	svc = find_service(ctx, iface, proto, name, type, domain);
	if (!svc) {
		svc = calloc(1, sizeof(*svc));
		if (!svc) {
			ERROR("mdns: out of memory for service '%s'", name);
			goto done;
		}
		svc->ifindex = iface;
		svc->proto   = proto;
		snprintf(svc->name,     sizeof(svc->name),     "%s", name);
		snprintf(svc->type,     sizeof(svc->type),     "%s", type);
		snprintf(svc->domain,   sizeof(svc->domain),   "%s", domain);
		snprintf(svc->hostname, sizeof(svc->hostname), "%s", hostname);
		LIST_INIT(&svc->txts);
		LIST_INSERT_HEAD(&ctx->services, svc, link);
	} else {
		free_txts(svc);
	}

	svc->port = port;

	/* Copy TXT records, skipping any that are not valid UTF-8 or contain
	 * bytes that are illegal in XML/YANG strings.  Apple devices sometimes
	 * embed raw binary tokens (device keys, protocol blobs) in TXT records;
	 * passing them to sr_set_item_str() would return EINVAL. */
	for (s = txtlist; s; s = avahi_string_list_get_next(s)) {
		uint8_t *data = avahi_string_list_get_text(s);
		size_t   len  = avahi_string_list_get_size(s);
		size_t   i;

		/* Validate: must be well-formed UTF-8 with no XML-illegal bytes */
		for (i = 0; i < len; ) {
			uint8_t b = data[i];
			int extra;

			if (b < 0x80) {
				/* ASCII: reject control chars invalid in XML */
				if ((b < 0x09) || (b > 0x0D && b < 0x20) || b == 0x7F)
					goto skip;
				i++;
				continue;
			}

			/* Multi-byte UTF-8 lead byte */
			if      ((b & 0xE0) == 0xC0) extra = 1;
			else if ((b & 0xF0) == 0xE0) extra = 2;
			else if ((b & 0xF8) == 0xF0) extra = 3;
			else goto skip; /* invalid lead byte */

			i++;
			for (; extra-- > 0; i++) {
				if (i >= len || (data[i] & 0xC0) != 0x80)
					goto skip; /* truncated sequence */
			}
		}

		t = calloc(1, sizeof(*t));
		if (!t)
			break;
		snprintf(t->val, sizeof(t->val), "%.*s", (int)len, (char *)data);
		LIST_INSERT_HEAD(&svc->txts, t, link);
		continue;
skip:
		DEBUG("mdns: skipping binary TXT record for '%s' (len=%zu)", name, len);
	}

	ds_push_resolver(ctx, svc, new_addr);

done:
	avahi_service_resolver_free(r);
}

static void service_browser_cb(AvahiServiceBrowser *b,
				AvahiIfIndex iface, AvahiProtocol proto,
				AvahiBrowserEvent event,
				const char *name, const char *type, const char *domain,
				AvahiLookupResultFlags flags,
				void *userdata)
{
	struct mdns_ctx *ctx = userdata;

	(void)b;
	(void)flags;

	switch (event) {
	case AVAHI_BROWSER_NEW:
		if (!avahi_service_resolver_new(ctx->client, iface, proto,
						name, type, domain,
						AVAHI_PROTO_UNSPEC, 0,
						resolver_cb, ctx))
			DEBUG("mdns: resolver_new(%s) failed: %s", name,
			      avahi_strerror(avahi_client_errno(ctx->client)));
		break;

	case AVAHI_BROWSER_REMOVE: {
		struct avahi_service *svc;
		char hostname[256];
		char svc_name[256];

		svc = find_service(ctx, iface, proto, name, type, domain);
		if (!svc)
			break;

		snprintf(hostname, sizeof(hostname), "%s", svc->hostname);
		snprintf(svc_name, sizeof(svc_name), "%s", svc->name);
		free_service(svc);

		/* Remove DS service entry if no other iface/proto instance remains */
		if (!svc_ds_entry_exists(ctx, hostname, svc_name)) {
			ds_delete_service(ctx, hostname, svc_name);

			/* Remove neighbor if it has no more services */
			if (!neighbor_has_services(ctx, hostname)) {
				ds_delete_neighbor(ctx, hostname);
				struct avahi_neighbor *n = find_neighbor(ctx, hostname);
				if (n)
					free_neighbor(n);
			}
		}

		sr_apply_changes(ctx->sr_ses, 0);
		break;
	}

	case AVAHI_BROWSER_ALL_FOR_NOW:
	case AVAHI_BROWSER_CACHE_EXHAUSTED:
	case AVAHI_BROWSER_FAILURE:
		break;
	}
}

static void type_browser_cb(AvahiServiceTypeBrowser *b,
			    AvahiIfIndex iface, AvahiProtocol proto,
			    AvahiBrowserEvent event,
			    const char *type, const char *domain,
			    AvahiLookupResultFlags flags,
			    void *userdata)
{
	struct mdns_ctx *ctx = userdata;

	(void)b;
	(void)flags;

	switch (event) {
	case AVAHI_BROWSER_NEW: {
		struct avahi_type_entry *te;

		/* Only create one browser per service type */
		LIST_FOREACH(te, &ctx->type_entries, link) {
			if (!strcmp(te->type, type))
				return;
		}

		te = calloc(1, sizeof(*te));
		if (!te)
			return;

		snprintf(te->type, sizeof(te->type), "%s", type);
		te->browser = avahi_service_browser_new(ctx->client,
							AVAHI_IF_UNSPEC,
							AVAHI_PROTO_UNSPEC,
							type, domain,
							0,
							service_browser_cb, ctx);
		if (!te->browser) {
			DEBUG("mdns: service_browser_new(%s) failed: %s", type,
			      avahi_strerror(avahi_client_errno(ctx->client)));
			free(te);
			return;
		}

		LIST_INSERT_HEAD(&ctx->type_entries, te, link);
		DEBUG("mdns: browsing service type %s", type);
		break;
	}

	case AVAHI_BROWSER_REMOVE: {
		struct avahi_type_entry *te;

		LIST_FOREACH(te, &ctx->type_entries, link) {
			if (!strcmp(te->type, type)) {
				avahi_service_browser_free(te->browser);
				LIST_REMOVE(te, link);
				free(te);
				break;
			}
		}
		break;
	}

	case AVAHI_BROWSER_ALL_FOR_NOW:
	case AVAHI_BROWSER_CACHE_EXHAUSTED:
	case AVAHI_BROWSER_FAILURE:
		break;
	}
}

/*
 * Check if mDNS is enabled in the running datastore.
 * Opens a temporary session to avoid disturbing the operational-DS session.
 * Returns true if enabled or if the check cannot be performed (fail-safe).
 */
static bool mdns_is_enabled(struct mdns_ctx *ctx)
{
	sr_session_ctx_t *sess = NULL;
	sr_data_t *data = NULL;
	const char *s;
	bool enabled = true; /* fail-safe: assume enabled */

	if (!ctx->sr_conn)
		return true;

	if (sr_session_start(ctx->sr_conn, SR_DS_RUNNING, &sess))
		return true;

	if (!sr_get_node(sess, "/infix-services:mdns/enabled", 0, &data) && data) {
		s = lyd_get_value(data->tree);
		if (s && !strcmp(s, "false"))
			enabled = false;
		sr_release_data(data);
	}

	sr_session_stop(sess);
	return enabled;
}

static void client_cb(AvahiClient *c, AvahiClientState state, void *userdata);

/*
 * Reconnect timer: fires MDNS_RECONN_DELAY seconds after AVAHI_CLIENT_FAILURE.
 * Frees the broken client and creates a fresh one.  libavahi's own AVAHI_CLIENT_NO_FAIL
 * reconnection can miss D-Bus NameOwnerChanged events; explicit free+recreate is
 * more reliable (same pattern used by mdns-alias).
 */
#define MDNS_RECONN_DELAY 3.0

static void reconn_cb(struct ev_loop *loop, ev_timer *w, int revents)
{
	struct mdns_ctx *ctx = (struct mdns_ctx *)
		((char *)w - offsetof(struct mdns_ctx, reconn_timer));
	int avahi_err;

	(void)loop;
	(void)revents;

	if (ctx->client) {
		avahi_client_free(ctx->client);
		ctx->client = NULL;
	}

	ctx->client = avahi_client_new(&ctx->poll_api, AVAHI_CLIENT_NO_FAIL,
				       client_cb, ctx, &avahi_err);
	if (!ctx->client)
		ERROR("mdns: failed to recreate avahi client: %s", avahi_strerror(avahi_err));
}

/*
 * Log-delay timer: fires MDNS_WARN_DELAY seconds after AVAHI_CLIENT_FAILURE.
 * Logs a single warning if mDNS is still enabled in the running config —
 * suppresses noise when the operator has simply disabled the mDNS service or
 * avahi is just restarting briefly.  Reconnection itself is handled by the
 * libavahi client (AVAHI_CLIENT_NO_FAIL) — we never give up.
 *
 * The delay must exceed libavahi's internal reconnect-poll interval (~5 s so
 * that a normal daemon restart cancels this timer before it fires.
 */
#define MDNS_WARN_DELAY 10.0

static void mdns_retry_cb(struct ev_loop *loop, ev_timer *w, int revents)
{
	struct mdns_ctx *ctx = (struct mdns_ctx *)
		((char *)w - offsetof(struct mdns_ctx, retry_timer));

	(void)loop;
	(void)revents;
	ctx->fail_count++;

	if (mdns_is_enabled(ctx))
		WARN("mdns: mDNS daemon not responding, will reconnect automatically");
}

static void client_cb(AvahiClient *c, AvahiClientState state, void *userdata)
{
	struct mdns_ctx *ctx = userdata;

	ctx->client = c;

	switch (state) {
	case AVAHI_CLIENT_S_RUNNING:
		if (ctx->fail_count > 0) {
			ev_timer_stop(ctx->loop, &ctx->reconn_timer);
			ev_timer_stop(ctx->loop, &ctx->retry_timer);
			NOTE("mdns: mDNS daemon reconnected");
			ctx->fail_count = 0;
		}
		INFO("mdns: client running");
		if (ctx->type_browser)
			break;  /* Already browsing */

		ctx->type_browser = avahi_service_type_browser_new(
			ctx->client,
			AVAHI_IF_UNSPEC, AVAHI_PROTO_UNSPEC,
			NULL, /* domain = NULL → "local" */
			0,
			type_browser_cb, ctx);
		if (!ctx->type_browser)
			ERROR("mdns: service_type_browser_new failed: %s",
			      avahi_strerror(avahi_client_errno(ctx->client)));
		break;

	case AVAHI_CLIENT_FAILURE:
		/*
		 * The daemon went away.  AVAHI_CLIENT_NO_FAIL means the client
		 * will reconnect automatically — we just need to clean up the
		 * now-invalid browsers so they're recreated on reconnect.
		 *
		 * Suppress the immediate ERROR; start a 2-second timer that
		 * will log only if the daemon stays down for 3 attempts (~6 s)
		 * and mDNS is enabled in the running config.
		 */
		if (!ev_is_active(&ctx->reconn_timer)) {
			ev_timer_init(&ctx->reconn_timer, reconn_cb, MDNS_RECONN_DELAY, 0.0);
			ev_timer_start(ctx->loop, &ctx->reconn_timer);
		}
		if (!ev_is_active(&ctx->retry_timer)) {
			ev_timer_init(&ctx->retry_timer, mdns_retry_cb, MDNS_WARN_DELAY, 0.0);
			ev_timer_start(ctx->loop, &ctx->retry_timer);
		}

		{
			struct avahi_type_entry *te;

			while (!LIST_EMPTY(&ctx->type_entries)) {
				te = LIST_FIRST(&ctx->type_entries);
				avahi_service_browser_free(te->browser);
				LIST_REMOVE(te, link);
				free(te);
			}
		}
		if (ctx->type_browser) {
			avahi_service_type_browser_free(ctx->type_browser);
			ctx->type_browser = NULL;
		}

		free_all(ctx);
		ds_clear_all(ctx);
		break;

	case AVAHI_CLIENT_S_COLLISION:
	case AVAHI_CLIENT_S_REGISTERING:
	case AVAHI_CLIENT_CONNECTING:
		break;
	}
}

/* --------------------------------------------------------------------------
 * Public interface
 * -------------------------------------------------------------------------- */

int mdns_ctx_init(struct mdns_ctx *ctx, struct ev_loop *loop, sr_conn_ctx_t *sr_conn)
{
	int avahi_err;

	memset(ctx, 0, sizeof(*ctx));
	ctx->loop    = loop;
	ctx->sr_conn = sr_conn;
	LIST_INIT(&ctx->neighbors);
	LIST_INIT(&ctx->services);
	LIST_INIT(&ctx->type_entries);

	/* Dedicated operational session for push writes (avoids sharing
	 * sr_query_ses which the journal thread also uses). */
	if (sr_session_start(sr_conn, SR_DS_OPERATIONAL, &ctx->sr_ses)) {
		ERROR("mdns: failed to start sysrepo session");
		return -1;
	}

	/* Wire up libev-backed AvahiPoll vtable */
	ctx->poll_api.userdata         = ctx;
	ctx->poll_api.watch_new        = watch_new;
	ctx->poll_api.watch_update     = watch_update;
	ctx->poll_api.watch_get_events = watch_get_events;
	ctx->poll_api.watch_free       = watch_free;
	ctx->poll_api.timeout_new      = timeout_new;
	ctx->poll_api.timeout_update   = timeout_update;
	ctx->poll_api.timeout_free     = timeout_free;

	ctx->client = avahi_client_new(&ctx->poll_api,
				       AVAHI_CLIENT_NO_FAIL,
				       client_cb, ctx,
				       &avahi_err);
	if (!ctx->client) {
		ERROR("mdns: client_new failed: %s", avahi_strerror(avahi_err));
		sr_session_stop(ctx->sr_ses);
		ctx->sr_ses = NULL;
		return -1;
	}

	INFO("mdns: mDNS neighbor monitor initialized");
	return 0;
}

void mdns_ctx_reconnect(struct mdns_ctx *ctx)
{
	struct avahi_type_entry *te;
	int avahi_err;

	if (!mdns_is_enabled(ctx)) {
		NOTE("mdns: mDNS is disabled, ignoring reconnect request");
		return;
	}

	NOTE("mdns: reconnecting on request");

	ev_timer_stop(ctx->loop, &ctx->reconn_timer);
	ev_timer_stop(ctx->loop, &ctx->retry_timer);
	ctx->fail_count = 0;

	/* Clean up browsers before freeing the client */
	while (!LIST_EMPTY(&ctx->type_entries)) {
		te = LIST_FIRST(&ctx->type_entries);
		avahi_service_browser_free(te->browser);
		LIST_REMOVE(te, link);
		free(te);
	}
	if (ctx->type_browser) {
		avahi_service_type_browser_free(ctx->type_browser);
		ctx->type_browser = NULL;
	}

	free_all(ctx);
	ds_clear_all(ctx);

	if (ctx->client) {
		avahi_client_free(ctx->client);
		ctx->client = NULL;
	}

	ctx->client = avahi_client_new(&ctx->poll_api, AVAHI_CLIENT_NO_FAIL,
				       client_cb, ctx, &avahi_err);
	if (!ctx->client)
		ERROR("mdns: failed to recreate avahi client: %s", avahi_strerror(avahi_err));
}

void mdns_ctx_exit(struct mdns_ctx *ctx)
{
	struct avahi_type_entry *te;

	if (ev_is_active(&ctx->reconn_timer))
		ev_timer_stop(ctx->loop, &ctx->reconn_timer);
	if (ev_is_active(&ctx->retry_timer))
		ev_timer_stop(ctx->loop, &ctx->retry_timer);

	/* Free browsers explicitly before freeing the client */
	while (!LIST_EMPTY(&ctx->type_entries)) {
		te = LIST_FIRST(&ctx->type_entries);
		avahi_service_browser_free(te->browser);
		LIST_REMOVE(te, link);
		free(te);
	}
	if (ctx->type_browser) {
		avahi_service_type_browser_free(ctx->type_browser);
		ctx->type_browser = NULL;
	}
	if (ctx->client) {
		avahi_client_free(ctx->client);
		ctx->client = NULL;
	}

	if (ctx->sr_ses) {
		ds_clear_all(ctx);
		sr_session_stop(ctx->sr_ses);
		ctx->sr_ses = NULL;
	}

	free_all(ctx);
	INFO("mdns: mDNS neighbor monitor stopped");
}
