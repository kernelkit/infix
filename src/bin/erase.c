/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <alloca.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "util.h"

static const char *prognm = "erase";
static int sanitize;

static int do_erase(const char *name)
{
	char *path;
	int rc = 0;

	path = cfg_adjust(name, NULL, sanitize);
	if (!path) {
		fprintf(stderr, ERRMSG "file not found.\n");
		rc = 1;
		goto out;
	}

	if (!yorn("Remove %s, are you sure?", path))
		goto out;

	if (remove(path)) {
		fprintf(stderr, ERRMSG "failed removing %s: %s\n", path, strerror(errno));
		rc = 11;
	}

out:
	free(path);
	return rc;
}

static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] PATH\n"
	       "\n"
	       "Options:\n"
	       "  -h         This help text\n"
	       "  -s         Sanitize paths for CLI use (restrict path traversal)\n"
	       "  -v         Show version\n", prognm);

	return rc;
}

int main(int argc, char *argv[])
{
	int c;

	while ((c = getopt(argc, argv, "hsv")) != EOF) {
		switch(c) {
		case 'h':
			return usage(0);
		case 's':
			sanitize = 1;
			break;
		case 'v':
			puts(PACKAGE_VERSION);
			return 0;
		}
	}

	if (optind >= argc)
		return usage(1);

	return do_erase(argv[optind++]);
}
