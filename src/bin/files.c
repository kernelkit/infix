/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <dirent.h>
#include <getopt.h>
#include <limits.h>
#include <stdlib.h>
#include <stdio.h>

#include "util.h"

static const char *prognm = "files";


int files(const char *path, const char *stripext)
{
	const struct dirent *d;
	DIR *dir;

	dir = opendir(path);
	if (!dir) {
		fprintf(stderr, "%s: %s: %s\n", prognm, path, strerror(errno));
		return -1;
	}

	while ((d = readdir(dir))) {
		char name[sizeof(d->d_name) + 1];

		/* only list regular files, skip dirs and dotfiles */
		if (d->d_type != DT_REG || d->d_name[0] == '.')
			continue;

		strlcpy(name, d->d_name, sizeof(name));
		if (stripext) {
			size_t pos = has_ext(name, stripext);

			if (pos)
				name[pos] = 0;
		}

		printf("%s\n", name);
	}

	return closedir(dir);
}


/*
 * List what matches base in dir, marking directories so the path can be
 * continued.  Only locations copy(1) and erase(1) accept are shown.
 */
static void list(const char *dir, const char *base)
{
	const struct dirent *d;
	char full[PATH_MAX];
	DIR *dp;

	dp = opendir(dir);
	if (!dp)
		return;

	while ((d = readdir(dp))) {
		int isdir;

		if (!strcmp(d->d_name, ".") || !strcmp(d->d_name, ".."))
			continue;

		/* Dotfiles only when one is being typed */
		if (d->d_name[0] == '.' && base[0] != '.')
			continue;

		if (strncmp(d->d_name, base, strlen(base)))
			continue;

		if (strlen(dir) + strlen(d->d_name) >= sizeof(full))
			continue;

		strlcpy(full, dir, sizeof(full));
		strlcat(full, d->d_name, sizeof(full));
		if (!path_traversable(full))
			continue;

		isdir = d->d_type == DT_DIR;
		if (d->d_type == DT_LNK || d->d_type == DT_UNKNOWN)
			isdir = fisdir(full);

		printf("%s%s\n", full, isdir ? "/" : "");
	}
	closedir(dp);
}

/* Complete an absolute path for the CLI */
static int complete(const char *word)
{
	char dir[PATH_MAX];
	const char *base;
	char *slash, *real;

	/* Absolute paths only, and copy(1) refuses '..' in any case */
	if (word[0] != '/' || strstr(word, ".."))
		return 0;

	if (strlen(word) >= sizeof(dir))
		return 0;
	strlcpy(dir, word, sizeof(dir));

	slash = strrchr(dir, '/');
	base = word + (slash - dir) + 1;
	slash[1] = 0;

	/* A symlink may lead out, the directory read must be allowed */
	real = realpath(dir, NULL);
	if (!real)
		return 0;

	if (!path_traversable(real)) {
		free(real);
		return 0;
	}
	free(real);

	list(dir, base);

	return 0;
}


static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] PATH [EXT]\n"
	       "\n"
	       "Options:\n"
	       "  -c PATH    Complete an absolute path, for CLI use\n"
	       "  -h         This help text\n"
	       "  -v         Show version\n", prognm);

	return rc;
}

int main(int argc, char *argv[])
{
	const char *path = NULL, *ext = NULL;
	int c;

	while ((c = getopt(argc, argv, "c:hv")) != EOF) {
		switch(c) {
		case 'c':
			return complete(optarg);
		case 'h':
			return usage(0);
		case 'v':
			puts(PACKAGE_VERSION);
			return 0;
		}
	}

	if (optind >= argc)
		return usage(1);

	path = argv[optind++];
	if (optind < argc)
		ext = argv[optind++];

	return files(path, ext);
}
