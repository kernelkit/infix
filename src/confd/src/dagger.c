/* SPDX-License-Identifier: BSD-3-Clause */

#include <libite/lite.h>
#include <srx/common.h>

#include "core.h"
#include "dagger.h"

#define PATH_ACTION_ "%s/%d/action/%s/%s"

static FILE *dagger_fopen(struct dagger *d, int gen, const char *action,
			  const char *node, unsigned char prio,
			  const char *script)
{
	char *path = NULL;
	const char *ext;
	FILE *fp = NULL;

	if (prio > 99) {
		errno = ERANGE;
		return NULL;
	}

	if (fmkpath(0755, PATH_ACTION_, d->path, gen, action, node))
		return NULL;

	if (asprintf(&path, PATH_ACTION_"/%02u-%s", d->path, gen, action, node, prio, script) == -1)
		return NULL;

	if (!fexist(path)) {
		fp = fopen(path, "w");
		if (!fp)
			goto fail;

		ext = rindex(script, '.');
		if (ext) {
			if (!strcmp(ext, ".sh"))
				fputs("#!/bin/sh\n\n", fp);
			else if (!strcmp(ext, ".bridge"))
				fputs("#!/sbin/bridge -batch\n\n", fp);
			else if (!strcmp(ext, ".ip"))
				fputs("#!/sbin/ip -batch\n\n", fp);
			else if (!strcmp(ext, ".sysctl"))
				fputs("#!/sbin/sysctl -p\n\n", fp);
		}

		if (fchmod(fileno(fp), 0774)) {
			fclose(fp);
			fp = NULL;
		}
	} else {
		fp = fopen(path, "a");
		if (!fp)
			goto fail;
	}
fail:
	free(path);
	return fp;
}

FILE *dagger_fopen_next(struct dagger *d, const char *action, const char *node,
			unsigned char prio, const char *script)
{
	return dagger_fopen(d, d->next, action, node, prio, script);
}

FILE *dagger_fopen_current(struct dagger *d, const char *action, const char *node,
			   unsigned char prio, const char *script)
{
	if (d->current < 0) {
		errno = EUNATCH;
		return NULL;
	}

	return dagger_fopen(d, d->current, action, node, prio, script);
}

FILE *dagger_fopen_net_init(struct dagger *d, const char *node, enum netdag_init order,
			    const char *script)
{
	return dagger_fopen_next(d, "init", node, order, script);
}

FILE *dagger_fopen_net_exit(struct dagger *d, const char *node, enum netdag_exit order,
			    const char *script)
{
	return dagger_fopen_current(d, "exit", node, order, script);
}

int dagger_add_dep(const struct dagger *d, const char *depender, const char *dependee)
{
	char link[strlen(d->path) + strlen(depender) + strlen(dependee) + 16];
	char target[strlen(dependee) + 16];
	char path[strlen(dependee) + 16];
	ssize_t len;

	/*
	 * Some callbacks may run twice, double check symlink, if it
	 * exists already and points to the same target, we're OK.
	 */
	snprintf(target, sizeof(target), "../%s", dependee);
	snprintf(link, sizeof(link), "%s/%d/dag/%s/%s", d->path, d->next, depender, dependee);

	len = readlink(link, path, sizeof(path));
	if (len > 0) {
		path[len] = 0;
		if (strcmp(target, path)) {
			ERROR("Dagger dependency already exists %s -> %s", target, path);
			return errno = EEXIST;
		}

		return 0;	/* same, ignore */
	}

	return symlink(target, link);
}

int dagger_add_node(struct dagger *d, const char *node)
{
	return fmkpath(0755, "%s/%d/dag/%s", d->path, d->next, node);
}

int dagger_abandon(struct dagger *d)
{
	int exitcode;

	if (!d->next_fp)
		return 0;

	fprintf(d->next_fp, "%d\n", d->next);
	fclose(d->next_fp);
	d->next_fp = NULL;

	exitcode = systemf("dagger -C %s abandon", d->path);
	DEBUG("dagger(%d->%d): abandon: exitcode=%d\n",
	      d->current, d->next, exitcode);

	return exitcode;
}

static int dagger_evolve(struct dagger *d)
{
	int exitcode;

	fprintf(d->next_fp, "%d\n", d->next);
	fclose(d->next_fp);
	d->next_fp = NULL;

	exitcode = systemf("dagger -C %s evolve", d->path);
	DEBUG("dagger(%d->%d): evolve: exitcode=%d\n",
	      d->current, d->next, exitcode);

	return exitcode;
}

static int dagger_prune(struct dagger *d)
{
	int exitcode;

	exitcode = systemf("dagger -C %s prune", d->path);
	DEBUG("dagger(%d->%d): prune: exitcode=%d\n",
	      d->current, d->next, exitcode);

	return exitcode;
}

int dagger_evolve_or_abandon(struct dagger *d)
{
	int exitcode, err;

	if (!d->next_fp)
		return 0;

	exitcode = dagger_evolve(d);
	dagger_prune(d);
	if (!exitcode)
		return 0;

	err = exitcode;

	exitcode = systemf("dagger -C %s abandon", d->path);
	DEBUG("dagger(%d->%d): Evolve failed, abandon: exitcode=%d\n",
	      d->current, d->next, exitcode);

	return err;
}

int dagger_is_bootstrap(struct dagger *d)
{
	return d->next == 0;
}

int dagger_claim(struct dagger *d, const char *path)
{
	char lnk[strlen(path) + 32];

	memset(d, 0, sizeof(*d));

	if (mkpath(path, 0755))
		return -1;

	d->next_fp = fopenf("wx", "%s/next", path);
	if (!d->next_fp) {
		ERROR("Transaction already in progress");
		return -1;
	}

	if (readdf(&d->current, "%s/current", path)) {
		d->current = -1;
	} else {
		if (fmkpath(0755, "%s/%d/action/exit", path, d->current))
			return -1;
		snprintf(lnk, sizeof(lnk), "%s/%d/action/exit/order", path, d->current);
		erase(lnk);
		if (symlink("../../top-down-order", lnk))
			return -1;
	}

	d->next = d->current + 1;
	if (fmkpath(0755, "%s/%d/action/init", path, d->next))
		return -1;
	if (fmkpath(0755, "%s/%d/skip", path, d->next))
		return -1;
	snprintf(lnk, sizeof(lnk), "%s/%d/action/init/order", path, d->next);
	if (symlink("../../bottom-up-order", lnk) && errno != EEXIST)
		return -1;

	strlcpy(d->path, path, sizeof(d->path));
	return 0;
}
