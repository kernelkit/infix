/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <dirent.h>
#include <getopt.h>
#include <stdio.h>

#include "util.h"

static const char *prognm = "files";


int files(const char *path, const char *stripext)
{
	const struct dirent *d;
	DIR *dir;

	dir = opendir(path);
	if (!dir) {
		fprintf(stderr, ERRMSG "%s", strerror(errno));
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


static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] PATH [EXT]\n"
	       "\n"
	       "Options:\n"
	       "  -h         This help text\n"
	       "  -v         Show version\n", prognm);

	return rc;
}

int main(int argc, char *argv[])
{
	const char *path = NULL, *ext = NULL;
	int c;

	while ((c = getopt(argc, argv, "hv")) != EOF) {
		switch(c) {
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
