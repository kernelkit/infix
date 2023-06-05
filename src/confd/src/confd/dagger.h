#ifndef _CONFD_DAGGER_H
#define _CONFD_DAGGER_H

#include <limits.h>
#include <stdio.h>

struct dagger {
	int current, next;
	FILE *next_fp;

	char path[PATH_MAX];
};

FILE *dagger_fopen_next(struct dagger *d, const char *action, const char *node,
			unsigned char prio, const char *script);
FILE *dagger_fopen_current(struct dagger *d, const char *action, const char *node,
			   unsigned char prio, const char *script);

int dagger_add_dep(struct dagger *d, const char *depender, const char *dependee);
int dagger_add_node(struct dagger *d, const char *node);
int dagger_abandon(struct dagger *d);
int dagger_evolve(struct dagger *d);
int dagger_evolve_or_abandon(struct dagger *d);

int dagger_claim(struct dagger *d, const char *path);

#endif	/* _CONFD_DAGGER_H */
