#include <libite/lite.h>

#include "core.h"
#include "dagger.h"
#include "../lib/common.h"

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

	if (systemf("mkdir -p " PATH_ACTION_, d->path, gen, action, node))
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

int dagger_add_dep(struct dagger *d, const char *depender, const char *dependee)
{
	return systemf("ln -s ../%s %s/%d/dag/%s", dependee,
		       d->path, d->next, depender);
}

int dagger_add_node(struct dagger *d, const char *node)
{
	return systemf("mkdir -p %s/%d/dag/%s", d->path, d->next, node);
}

int dagger_abandon(struct dagger *d)
{
	fprintf(d->next_fp, "%d\n", d->next);
	fclose(d->next_fp);
	return systemf("dagger -C %s abandon", d->path);
}

int dagger_evolve(struct dagger *d)
{
	fprintf(d->next_fp, "%d\n", d->next);
	fclose(d->next_fp);
	return systemf("dagger -C %s evolve", d->path);
}

int dagger_evolve_or_abandon(struct dagger *d)
{
	int err;

	err = dagger_evolve(d);
	if (!err)
		return 0;

	systemf("dagger -C %s abandon", d->path);
	return err;
}

void dagger_skip_iface(struct dagger *d, const char *ifname)
{
	touchf("%s/%d/skip/%s", d->path, d->next, ifname);
}

int dagger_should_skip(struct dagger *d, const char *ifname)
{
	return fexistf("%s/%d/skip/%s", d->path, d->next, ifname);
}

int dagger_should_skip_current(struct dagger *d, const char *ifname)
{
	return fexistf("%s/%d/skip/%s", d->path, d->current, ifname);
}


int dagger_claim(struct dagger *d, const char *path)
{
	int err;

	memset(d, 0, sizeof(*d));

	err = systemf("mkdir -p %s", path);
	if (err)
		return err;

	d->next_fp = fopenf("wx", "%s/next", path);
	if (!d->next_fp) {
		ERROR("Transaction already in progress");
		return -1;
	}

	if (readdf(&d->current, "%s/current", path)) {
		d->current = -1;
	} else {
		err = systemf("mkdir -p %s/%d/action/exit"
			      " && "
			      "ln -s ../../top-down-order %s/%d/action/exit/order",
			      path, d->current, path, d->current);
		if (err)
			return err;
	}

	d->next = d->current + 1;
	err = systemf("mkdir -p %s/%d/action/init"
		      " && "
		      "mkdir -p %s/%d/skip"
		      " && "
		      "ln -s ../../bottom-up-order %s/%d/action/init/order",
		      path, d->next, path, d->next, path, d->next);
	if (err)
		return err;

	strlcpy(d->path, path, sizeof(d->path));
	return 0;
}
