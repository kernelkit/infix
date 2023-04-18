#include <err.h>
#include <libgen.h>
#include <search.h>
#include <stdlib.h>
#include <sysexits.h>
#include <net/if.h>
#include <libite/lite.h>
#include <libite/queue.h>

#define _PATH_NET "/run/net"
#define dbg(fmt, args...) if (debug)   warnx(fmt, ##args)
#define log(fmt, args...) if (verbose) warnx(fmt, ##args)

typedef enum {
	INVAL = -1,
	DO = 0,
	UP = 1,
	DOWN = 2,
} cmd_t;

struct iface {
        TAILQ_ENTRY(iface) link;
        char ifname[IFNAMSIZ];
};

static TAILQ_HEAD(iflist, iface) iface_list = TAILQ_HEAD_INITIALIZER(iface_list);

static int debug;
static int verbose;
static int dep;

static FILE *ip, *bridge;
static char *prognm;

static void addif(char *ifname)
{
	struct iface *entry;

	entry = malloc(sizeof(*entry));
	if (!entry)
		err(1, "malloc");

	strlcpy(entry->ifname, ifname, sizeof(entry->ifname));
	TAILQ_INSERT_TAIL(&iface_list, entry, link);
}

static int findif(char *ifname)
{
	struct iface *iface;

	TAILQ_FOREACH(iface, &iface_list, link) {
		if (strcmp(iface->ifname, ifname))
			continue;

		return 1;
	}

	return 0;
}

static void freeifs(void)
{
	struct iface *iface, *tmp;

	TAILQ_FOREACH_SAFE(iface, &iface_list, link, tmp) {
		TAILQ_REMOVE(&iface_list, iface, link);
		free(iface);
	}
}

static void savedep(char *ipath)
{
	char line[20];
	char *ifname;
	char *path;
	FILE *fp;

	path = strdupa(ipath);
	if (!path)
		return;

	ifname = basename(path);
	path = dirname(path);

	fp = fopenf("a+", "%s/deps", path);
	if (!fp)
		return;

	(void)fseek(fp, 0L, SEEK_SET);
	while (fgets(line, sizeof(line), fp)) {
		if (!strcmp(chomp(line), ifname))
			goto done;
	}

	dep++;
	fprintf(fp, "%s\n", ifname);
done:
	fclose(fp);
}

static int save_rdeps(const char *path, char *gen)
{
	return systemf("sed '1!G;h;$!d' < %s/%s/deps >%s/%s/rdeps", path, gen, path, gen);
}

static int pipeit(FILE *pp, const char *action)
{
	char line[256];
	FILE *fp;

	fp = fopen(action, "r");
	if (!fp)
		return 0;	/* nop */

	log("running %s ...", action);

	while (fgets(line, sizeof(line), fp)) {
		dbg("%s: read line: %s", action, line);
		fputs(line, pp);
	}

	return fclose(fp);
}

static int run(const char *action)
{
	char *ptr;
	int rc;

	ptr = strrchr(action, '.');
	if (!ptr) {
		warnx("invalid action script: '%s'", action);
		return 0;
	}

	if (strcmp(action, ".ip"))
		return pipeit(ip, action);
	if (strcmp(action, ".bridge"))
		return pipeit(bridge, action);

	 /* other actions are plain shell scripts (.sh) */
	log("running %s ...", action);

	rc = systemf("%s", action);
	if (rc)
		warn("failed %s, rc %d", action, rc);
	return 0;
}

static int dir_filter(const char *file)
{
	char *files[] =  {
		"deps",
		"rdeps",
		"admin-state",
	};
	size_t i;

	for (i = 0; i < NELEMS(files); i++) {
		if (!strcmp(file, files[i]))
			return 0;
	}

	return 1;
}

static int deps(char *ipath, char *ifname, const char *action)
{
	char path[strlen(ipath) + 42];
	int num, rc = -1;
	char **files;
	char *cmd;

	snprintf(path, sizeof(path), "%s/deps", ipath);
	num = dir(path, NULL, dir_filter, &files, 0);
	for (int i = 0; i < num; i++) {
		char dpath[sizeof(path) + strlen(files[i])];
		char *ifnm = files[i];
		char *rp;

		snprintf(dpath, sizeof(dpath), "%s/%s", path, ifnm);
		rp = realpath(dpath, NULL);
		if (!rp)
			continue;

		deps(rp, ifnm, action);
		free(ifnm);
		free(rp);
	}

	savedep(ipath);
	if (findif(ifname))
		return 0;

	snprintf(path, sizeof(path), "%s/%s", ipath, action);
	cmd = realpath(path, NULL);
	if (!cmd || access(cmd, F_OK)) {
		if (errno == ENOENT || errno == ENOTDIR)
			rc = 0;	/* no action for this interface */
		goto done;
	}

	rc = run(cmd);
done:
	free(cmd);
	addif(ifname);

	return rc;
}

static int iter(char *path, size_t len, const char *action)
{
	char **files;
	int rc = 0;
	int num;

	num = dir(path, NULL, dir_filter, &files, 0);

	for (int j = 0; j < num; j++) {
		char *ifname = files[j];
		char ipath[len];

		snprintf(ipath, sizeof(ipath), "%s/%s", path, ifname);
		dbg("Calling deps(%s, %s, %s)", ipath, ifname, action);
		rc += deps(ipath, ifname, action);
		dbg("rc => %d", rc);
		free(ifname);
	}

	freeifs();

	return rc;
}

static int rdeps(char *path, size_t len, const char *action)
{
	char cmd[len + 20 + strlen(action)];
	char ifname[20];
	int rc = 0;
	FILE *fp;

	fp = fopenf("r", "%s/rdeps", path);
	if (!fp)
		return 0;	/* no deps in prev. generation */

	while (fgets(ifname, sizeof(ifname), fp)) {
		snprintf(cmd, sizeof(cmd), "%s/%s/%s", path, chomp(ifname), action);
		if (access(cmd, F_OK)) {
			dbg("skipping %s, errno %d: %s", cmd, errno, strerror(errno));
			continue;
		}
		rc += run(cmd);
	}

	return rc;
}

static int deactivate(const char *net, char *gen)
{
	char path[strlen(net) + strlen(gen) + 5 + IFNAMSIZ];
	char *action[] = {
		"exit.bridge",
		"exit.ip",
		"exit-ethtool.sh",
	};
	int rc = 0;

	snprintf(path, sizeof(path), "%s/%s", net, gen);
	for (size_t i = 0; i < NELEMS(action); i++)
		rc += rdeps(path, sizeof(path), action[i]);

	return rc;
}

static int activate(const char *net, char *gen)
{
	char path[strlen(net) + strlen(gen) + 5 + IFNAMSIZ];
	char *action[] = {
		"init-ethtool.sh",
		"init.ip",
		"init.bridge",
	};
	int rc = 0;

	snprintf(path, sizeof(path), "%s/%s", net, gen);
	for (size_t i = 0; i < NELEMS(action); i++)
		rc += iter(path, sizeof(path), action[i]);

	return rc;
}

static int load_gen(const char *net, char *gen, char *buf, size_t len)
{
	FILE *fp;

	fp = fopenf("r", "%s/%s", net, gen);
	if (!fp)
		return EX_OSFILE;
	if (!fgets(buf, len, fp))
		return EX_IOERR;
	fclose(fp);
	chomp(buf);

	return 0;
}

static int save_gen(const char *net, char *gen, char *buf)
{
	FILE *fp;

	fp = fopenf("w", "%s/%s", net, gen);
	if (!fp)
		return -1;
	fprintf(fp, "%s\n", buf);
	fclose(fp);

	return 0;
}

static void pipe_init(void)
{
	ip = popen("ip -batch -", "w");
	if (!ip)
		err(1, "failed starting ip command pipe");

	bridge = popen("bridge -batch -", "w");
	if (!bridge)
		err(1, "failed starting bridge command pipe");
}

static const char *getnet(void)
{
	const char *net = _PATH_NET;

	if (getenv("NET_DIR"))
		net = getenv("NET_DIR");
	dbg("net directory %s", net);
	if (access(net, X_OK)) {
		if (makedir(net, 0755))
			err(1, "makedir");
	}

	return net;
}

/* build list from current generation */
static void getifs(void)
{
	const char *net = getnet();
	char **files;
	int num;

	num = dir(net, NULL, dir_filter, &files, 0);
	for (int i = 0; i < num; i++)
		addif(files[i]);
}

static char *ifadmin(const char *ifname, char *buf, size_t len)
{
	const char *net = getnet();

	if (!net)
		return NULL;

	snprintf(buf, len, "%s/%s/admin-state", net, ifname);
	return buf;
}

/* is the interface eligible for being taken up/down? */
static int allowed(const char *ifname)
{
	char buf[128];
	int rc = 1;
	FILE *fp;

	if (!ifadmin(ifname, buf, sizeof(buf)))
		goto fail;

	fp = fopen(buf, "r");
	if (fp) {
		if (fgets(buf, sizeof(buf), fp)) {
			chomp(buf);
			if (!strcmp(buf, "disabled"))
				rc = 0;
		}
		fclose(fp);
	}
fail:
	return rc;
}

static int ifupdown(int updown)
{
	struct iface *iface;
	int rc = 0;

	if (TAILQ_EMPTY(&iface_list))
		getifs();

	TAILQ_FOREACH(iface, &iface_list, link) {
		const char *action;
		int result;

		if (!allowed(iface->ifname))
			continue;

		action = updown ? "up" : "down";
		result = systemf("ip link set %s %s", iface->ifname, action);
		if (!result) {
			char buf[128];
			FILE *fp;

			if (!ifadmin(iface->ifname, buf, sizeof(buf)))
				continue;

			fp = fopen(buf, "w");
			if (fp) {
				fprintf(fp, "%s\n", action);
				fclose(fp);
			}
		}

		rc += result;
	}

	return rc;
}

static int activate_next(void)
{
	const char *net = getnet();
	char curr[512], next[512];
	int rc;

	pipe_init();

	if ((rc = load_gen(net, "next", next, sizeof(next)))) {
		if (rc == EX_IOERR)
			warnx("missing next generation");
		exit(0); /* nothing to do */
	}

	if ((rc = load_gen(net, "gen", curr, sizeof(curr)))) {
		if (rc == EX_IOERR)
			errx(rc, "missing current generation");
		/* no current generation */
	} else {
		rc = deactivate(net, curr);
		if (rc)
			err(1, "failed deactivating current generation");
	}

	rc = activate(net, next);
	if (rc)
		err(1, "failed activating next generation");

	if (save_rdeps(net, next))
		err(1, "failed saving interface deps in %s", next);

	if (save_gen(net, "gen", next))
		err(1, "next generation applied, failed current");

	if (fremove("%s/next", net))
		err(1, "failed removing %s/next", net);

	return 0;
}

static int act(cmd_t cmd)
{
	switch (cmd) {
	case DO:
		return activate_next();
	case UP:
		return ifupdown(1);
	case DOWN:
		return ifupdown(0);
	default:
		break;
	}

	freeifs();

	return EX_USAGE;
}

cmd_t transform(char *arg0)
{
	prognm = strrchr(arg0, '/');
	if (prognm)
		prognm++;
	else
		prognm = arg0;

	if (!strcmp(prognm, "ifup"))
		return UP;
	if (!strcmp(prognm, "ifdown"))
		return DOWN;

	return INVAL;
}

static int usage(int code)
{
	printf("Usage: %s [-dh] [do | (up | down [ifname ...])]\n"
	       "\n"
	       "Options:\n"
	       "  -a      Act on all interfaces, ignored, for compat only.\n"
	       "  -d      Debug\n"
	       "  -h      This help text\n"
	       "  -v      Verbose, show actions taken\n"
	       "\n"
	       "Commands:\n"
	       "  do      Activate next network generation\n"
	       "  up      Bring up one/many or all interfaces in the current generation\n"
	       "  down    Take down one/many or all interfaces in the current generation\n"
	       "\n"
	       "Args:\n"
	       "  ifname  Zero, one, or more interface names to act on.\n"
	       "\n", prognm);

	return code;
}

int main(int argc, char *argv[])
{
	cmd_t cmd;
	int c;

	cmd = transform(argv[0]);

	while ((c = getopt(argc, argv, "adhv")) != EOF) {
		switch (c) {
		case 'a':
			/* compat with ifup/ifdown */
			break;
		case 'd':
			debug = 1;
			break;
		case 'h':
			return usage(0);
		case 'v':
			verbose = 1;
			break;
		default:
			return usage(1);
		}
	}

	for (c = optind; c < argc; c++) {
		if (cmd == INVAL) {
			if (!strcmp("do", argv[c]) || !strcmp("apply", argv[c]))
				cmd = DO;
			if (!strcmp("up", argv[c]))
				cmd = UP;
			if (!strcmp("down", argv[c]))
				cmd = DOWN;

			continue;
		}

		addif(argv[c]);
	}

	if (cmd == INVAL)
		return usage(EX_USAGE);

	return act(cmd);
}
