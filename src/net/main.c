#include <err.h>
#include <libgen.h>
#include <stdlib.h>
#include <net/if.h>
#include <libite/lite.h>

#define _PATH_NET "/run/net"
#define DEBUG 1
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

static int deps(char *ipath, char *ifname, char *action)
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

static int activate(const char *net, char *gen)
{
	char path[strlen(net) + strlen(gen) + 5 + IFNAMSIZ];
	char *action[] = {
		"ip-addr.dn",
		"ip-link.dn",
		"ip-link.up",
		"ip-addr.up",
	};
	char **files;
	int rc = 0;

	snprintf(path, sizeof(path), "%s/%s", net, gen);

	for (size_t i = 0; i < NELEMS(action); i++) {
		char *act = action[i];
		int num;

		num = dir(path, NULL, NULL, &files, 0);
		if_alloc(num);

		for (int j = 0; j < num; j++) {
			char ipath[sizeof(path)];
			char *ifname = files[j];

			snprintf(ipath, sizeof(ipath), "%s/%s", path, ifname);
			rc += deps(ipath, ifname, act);
			free(ifname);
		}

		if_free();
	}

	return rc;
}

int main(void)
{
	const char *net = _PATH_NET;
	char next[512];
	FILE *fp;
	int rc;

	if (getenv("NET_DIR"))
		net = getenv("NET_DIR");
	if (access(net, X_OK)) {
		if (makedir(net, 0755))
			err(1, "makedir");
	}

	fp = fopenf("r", "%s/next", net);
	if (!fp)
		exit(0); /* nothing to do */

	if (!fgets(next, sizeof(next), fp))
		err(1, "missing next generation");
	fclose(fp);

	rc = activate(net, chomp(next));
	if (rc)
		err(1, "failed activating next generation");

	fp = fopenf("w", "%s/gen", net);
	if (!fp)
		err(1, "next generation applied, failed current");

	fprintf(fp, "%s\n", next);
	fclose(fp);

	return fremove("%s/next", net);
}
