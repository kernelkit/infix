#include <err.h>
#include <libgen.h>
#include <stdlib.h>
#include <sysexits.h>
#include <net/if.h>
#include <libite/lite.h>

#define _PATH_NET "/run/net"
#define dbg(fmt, args...) if (debug)   warnx(fmt, ##args)
#define log(fmt, args...) if (verbose) warnx(fmt, ##args)

static char **handled;
static int if_num;
static int debug;
static int verbose;
static int dep;

static FILE *ip, *bridge;
extern char *__progname;


static void if_alloc(int num)
{
	handled = calloc(num, sizeof(char *));
	if (!handled)
		err(1, "calloc");
	if_num = num;
}

static void if_free()
{
	for (int i = 0; i < if_num; i++) {
		if (handled[i])
			free(handled[i]);
	}
	free(handled);
}

static void if_done(char *ifname)
{
	for (int i = 0; i < if_num; i++) {
		if (handled[i]) {
			if (strcmp(handled[i], ifname))
				continue;
			return;
		}
		handled[i] = strdup(ifname);
		break;
	}
}

static int if_find(char *ifname)
{
	for (int i = 0; i < if_num; i++) {
		if (handled[i] && !strcmp(handled[i], ifname))
			return 1;
	}
	return 0;
}

static void savedep(char *ipath)
{
	char *ifname = basename(ipath);
	char *path = dirname(ipath);
	char buf[20];
	FILE *fp;

	if (!strcmp(ifname, "deps"))
		return;

	fp = fopenf("a+", "%s/deps", path);
	if (!fp)
		return;

	(void)fseek(fp, 0L, SEEK_SET);
	while (fgets(buf, sizeof(buf), fp)) {
		if (!strcmp(chomp(buf), ifname))
			goto done;
	}

	dep++;
	fprintf(fp, "%s\n", ifname);
done:
	fclose(fp);
}

static int save_rdeps(const char *path, char *gen)
{
	char ipath[strlen(path) + strlen(gen) + 16 + 4];
#if 0
	char **list;
	int i, num;

	snprintf(ipath, sizeof(ipath), "%s/%s", path, gen);
	num = dir(ipath, NULL, NULL, &list, 0);
	if (!num)
		return -1;

	for (i = 0; i < num; i++) {
		snprintf(ipath, sizeof(ipath), "%s/%s/%s", path, gen, list[i]);
		if (access(ipath, X_OK))
			continue;

		savedep(ipath);
	}
#endif
	snprintf(ipath, sizeof(ipath), "%s/%s", path, gen);
	return systemf("sed '1!G;h;$!d' < %s/deps >%s/rdeps", ipath, ipath);
}

static int pipeit(FILE *pp, const char *action)
{
	char line[256];
	FILE *fp;

	fp = fopen(action, "r");
	if (!fp)
		return 0;	/* nop */

	log("running %s ...", action);

	while (fgets(line, sizeof(line), fp))
		fputs(line, pp);

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

static int deps(char *ipath, char *ifname, const char *action)
{
	char path[strlen(ipath) + 42];
	int num, rc = -1;
	char **files;
	char *cmd;

	snprintf(path, sizeof(path), "%s/deps", ipath);
	num = dir(path, NULL, NULL, &files, 0);
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
	if (if_find(ifname))
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
	if_done(ifname);
	
	return rc;
}

static int iter(char *path, size_t len, const char *action)
{
	char **files;
	int rc = 0;
	int num;

	num = dir(path, NULL, NULL, &files, 0);
	if_alloc(num);

	for (int j = 0; j < num; j++) {
		char *ifname = files[j];
		char ipath[len];

		snprintf(ipath, sizeof(ipath), "%s/%s", path, ifname);
		dbg("Calling deps(%s, %s, %s)", ipath, ifname, action);
		rc += deps(ipath, ifname, action);
		dbg("rc => %d", rc);
		free(ifname);
	}

	if_free();

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

static int usage(int code)
{
	printf("Usage: %s [-dh]\n"
	       "\n"
	       "Options:\n"
	       "  -d      Debug\n"
	       "  -h      This help text\n"
	       "  -v      Verbose, show actions taken\n"
	       "\n", __progname);

	return code;
}

int main(int argc, char *argv[])
{
	const char *net = _PATH_NET;
	char curr[512], next[512];
	int rc, c;

	while ((c = getopt(argc, argv, "dhv")) != EOF) {
		switch (c) {
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

	if (getenv("NET_DIR"))
		net = getenv("NET_DIR");
	if (access(net, X_OK)) {
		if (makedir(net, 0755))
			err(1, "makedir");
	}

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
