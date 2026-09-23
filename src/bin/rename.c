/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <errno.h>
#include <getopt.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <limits.h>
#include <sys/stat.h>
#include <unistd.h>

#include "util.h"

static const char *prognm = "rename";
static int sanitize;
static int force;

/* Create the directory a file is moving into, with search where read */
static int mkparent(const char *path)
{
	char dir[PATH_MAX];
	mode_t mode;
	int len;

	len = dirlen(path);
	if (len <= 0 || (size_t)len >= sizeof(dir))
		return 0;

	strlcpy(dir, path, (size_t)len + 1);
	if (fisdir(dir))
		return 0;

	mode = path_mode(path) ?: 0660;
	mode |= (mode & 0444) >> 2;

	if (mkpath(dir, mode))
		return -1;

	return chmod(dir, mode);
}

static int do_rename(const char *from, const char *to)
{
	char *src = NULL, *dst = NULL;
	mode_t mode;
	int rc = 1;

	src = cfg_adjust(from, NULL, sanitize);
	if (!src || access(src, F_OK)) {
		fprintf(stderr, "%s: %s: no such file, or not an allowed path\n",
			prognm, from);
		goto out;
	}

	dst = cfg_adjust(to, from, sanitize);
	if (!dst) {
		fprintf(stderr, "%s: %s: not an allowed path\n", prognm, to);
		goto out;
	}

	if (!force && !access(dst, F_OK) && !yorn("Overwrite existing file %s", dst))
		goto out;

	if (mkparent(dst)) {
		fprintf(stderr, "%s: failed creating directory for %s: %s\n",
			prognm, dst, strerror(errno));
		goto out;
	}

	if (rename(src, dst)) {
		if (errno == EXDEV)
			fprintf(stderr, "%s: %s and %s are on different file systems,"
				" use copy and remove\n", prognm, src, dst);
		else
			fprintf(stderr, "%s: failed renaming %s: %s\n", prognm, src,
				strerror(errno));
		goto out;
	}

	/* Keep the mode the destination calls for, it may be served */
	mode = path_mode(dst);
	if (mode && chmod(dst, mode) && errno != EPERM)
		fprintf(stderr, "%s: failed setting mode 0%o on %s: %s\n", prognm,
			mode, dst, strerror(errno));

	rc = 0;
out:
	free(dst);
	free(src);

	return rc;
}

static int usage(int rc)
{
	printf("Usage: %s [OPTIONS] FROM TO\n"
	       "\n"
	       "Options:\n"
	       "  -f         Force, overwrite an existing file without asking\n"
	       "  -h         This help text\n"
	       "  -s         Sanitize paths for CLI use (restrict path traversal)\n"
	       "  -v         Show version\n", prognm);

	return rc;
}

int main(int argc, char *argv[])
{
	int c;

	while ((c = getopt(argc, argv, "fhsv")) != EOF) {
		switch(c) {
		case 'f':
			force = 1;
			break;
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

	if (argc - optind != 2)
		return usage(1);

	return do_rename(argv[optind], argv[optind + 1]);
}
