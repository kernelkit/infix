#include <libite/lite.h>

#include "core.h"
#include "dagger.h"

static FILE *dagger_fopen(struct dagger *d, int gen, const char *action,
			  const char *node, unsigned char prio,
			  const char *script)
{
	const char *ext;
	FILE *fp;
	int err;

	if (prio > 99)
		return NULL;

	err = systemf("mkdir -p %s/%d/action/%s/%s",
		      d->path, gen, action, node);
	if (err)
		return NULL;

	fp = fopenf("w", "%s/%d/action/%s/%s/%02u-%s",
		    d->path, gen, action, node, prio, script);
	if (!fp)
		return NULL;

	ext = rindex(script, '.');
	if (ext) {
		if (!strcmp(ext, ".sh"))
			fputs("#!/bin/sh\n\n", fp);
		else if (!strcmp(ext, ".bridge"))
			fputs("#!/sbin/bridge -batch\n\n", fp);
		else if (!strcmp(ext, ".ip"))
			fputs("#!/sbin/ip -batch\n\n", fp);
	}

	err = fchmod(fileno(fp), 0774);
	if (err) {
		fclose(fp);
		return NULL;
	}

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
	if (d->current < 0)
		return NULL;

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

	if (readdf(&d->current, "%s/current", path))
		d->current = -1;

	d->next = d->current + 1;

	strlcpy(d->path, path, sizeof(d->path));
	return 0;
}
