/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <alloca.h>
#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#include "util.h"

static const char *prognm = "erase";
static int sanitize;

static int do_erase(const char *path)
{
	char buf[PATH_MAX];
	const char *fn;

	fn = cfg_adjust(path, NULL, buf, sizeof(buf), sanitize);
	if (!fn) {
		fprintf(stderr, ERRMSG "file not found.\n");
		return 1;
	}

	if (!yorn("Remove %s, are you sure", path))
		return 0;

	if (remove(fn)) {
		fprintf(stderr, ERRMSG "failed removing %s: %s\n", path, strerror(errno));
		return 11;
	}

	return 0;
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
