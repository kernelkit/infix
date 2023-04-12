#include <err.h>
#include <libgen.h>
#include <stdlib.h>
#include <sysexits.h>
#include <net/if.h>
#include <libite/lite.h>

#define _PATH_NET "/run/net"
#define DEBUG 0
#define dbg(fmt, args...) if (DEBUG) warnx(fmt, ##args)

static char **handled;
static int if_num;


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

	if (if_find(ifname))
		return 0;

	snprintf(path, sizeof(path), "%s/%s", ipath, action);
	cmd = realpath(path, NULL);
	if (!cmd || access(cmd, X_OK)) {
		if (errno == ENOENT)
			rc = 0;	/* no action for this interface */
		goto done;
	}

	rc = systemf("%s", cmd);
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
		free(ifname);
	}

	if_free();

	return rc;
}

static int deactivate(const char *net, char *gen)
{
	char path[strlen(net) + strlen(gen) + 5 + IFNAMSIZ];
	char *action[] = {
		"ip.exit",
		"bridge.exit",
		"ethtool.exit",
	};
	int rc = 0;

	snprintf(path, sizeof(path), "%s/%s", net, gen);
	for (size_t i = 0; i < NELEMS(action); i++)
		rc += iter(path, sizeof(path), action[i]);

	return rc;
}

static int activate(const char *net, char *gen)
{
	char path[strlen(net) + strlen(gen) + 5 + IFNAMSIZ];
	char *action[] = {
		"ethtool.init",
		"bridge.init",
		"ip.init",
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

int main(void)
{
	const char *net = _PATH_NET;
	char curr[512], next[512];
	int rc;

	if (getenv("NET_DIR"))
		net = getenv("NET_DIR");
	if (access(net, X_OK)) {
		if (makedir(net, 0755))
			err(1, "makedir");
	}

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

	if (save_gen(net, "gen", next))
		err(1, "next generation applied, failed current");

	if (fremove("%s/next", net))
		err(1, "failed removing %s/next", net);

	return 0;
}
