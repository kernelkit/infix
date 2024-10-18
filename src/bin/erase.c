/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <alloca.h>
#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#include "util.h"

static const char *prognm = "erase";


static int do_erase(const char *path)
{
	char *fn;

	if (access(path, F_OK)) {
		size_t len = strlen(path) + 10;

		fn = alloca(len);
		if (!fn) {
			fprintf(stderr, ERRMSG "failed allocating memory.\n");
			return -1;
		}

		cfg_adjust(path, NULL, fn, len);
		if (access(fn, F_OK)) {
			fprintf(stderr, "No such file: %s\n", fn);
			return -1;
		}
	} else
		fn = (char *)path;

	if (!yorn("Remove %s, are you sure", fn))
		return 0;

	if (remove(fn)) {
		fprintf(stderr, ERRMSG "failed removing %s: %s\n", fn, strerror(errno));
		return -1;
	}

	return 0;
}

static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] PATH\n"
	       "\n"
	       "Options:\n"
	       "  -h         This help text\n"
	       "  -v         Show version\n", prognm);

	return rc;
}

int main(int argc, char *argv[])
{
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

	return do_erase(argv[optind++]);
}
