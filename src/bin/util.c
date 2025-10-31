/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <dirent.h>
#include <errno.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <termios.h>

#include "util.h"


static char rawgetch(void)
{
	struct termios saved, c;
	int key;

	if (tcgetattr(fileno(stdin), &saved) < 0)
		return -1;

	c = saved;
	c.c_lflag &= ~ICANON;
	c.c_lflag &= ~ECHO;
	c.c_cc[VMIN] = 1;
	c.c_cc[VTIME] = 0;

	if (tcsetattr(fileno(stdin), TCSANOW, &c) < 0) {
		tcsetattr(fileno(stdin), TCSANOW, &saved);
		return -1;
	}

	key = getchar();
	tcsetattr(fileno(stdin), TCSANOW, &saved);

	if (key == EOF)
		return -1;

	return (char)key;
}

int yorn(const char *fmt, ...)
{
	va_list ap;
	char ch;

	va_start(ap, fmt);
	vfprintf(stderr, fmt, ap);
	va_end(ap);

	fprintf(stderr, " (y/N)? ");
	ch = rawgetch();
	fprintf(stderr, "%c\n", ch);
	if (ch != 'y' && ch != 'Y')
		return 0;

	return 1;
}

int has_ext(const char *fn, const char *ext)
{
	size_t pos = strlen(fn);
	size_t len = strlen(ext);

	if (len < pos && !strcmp(&fn[pos - len], ext))
		return pos - len;
	return 0;
}

int dirlen(const char *path)
{
	const char *slash;

	slash = strrchr(path, '/');
	if (slash)
		return slash - path;

	return 0;
}

const char *basenm(const char *path)
{
	const char *slash;

	if (!path)
		return NULL;

	slash = strrchr(path, '/');
	if (slash)
		return slash[1] ? slash + 1 : NULL;

	return path;
}

static int path_allowed(const char *path)
{
	const char *accepted[] = {
		"/media/",
		"/cfg/",
		getenv("HOME"),
		NULL
	};

	for (int i = 0; accepted[i]; i++) {
		if (!strncmp(path, accepted[i], strlen(accepted[i])))
			return 1;
	}

	return 0;
}

char *cfg_adjust(const char *path, const char *template, bool sanitize)
{
	char *expanded = NULL, *resolved = NULL;
	const char *basename;
	int dlen;

	dlen = dirlen(path);
	basename = basenm(path) ? : basenm(template);
	if (!basename)
		goto err;

	if (sanitize) {
		if (strstr(path, "../"))
			goto err;

		if (path[0] == '/') {
			if (!path_allowed(path))
				goto err;
		}
	}

	if (asprintf(&expanded, "%s%.*s/%s%s",
		     path[0] == '/' ? "" : "/cfg/",
		     dlen, path,
		     basename,
		     strchr(basename, '.') ? "" : ".cfg") < 0)
		goto err;

	/* If file exists, resolve symlinks and verify still in whitelist */
	if (sanitize && !access(expanded, F_OK)) {
		resolved = realpath(expanded, NULL);
		if (!resolved || !path_allowed(resolved))
			goto err;

		free(expanded);
		expanded = resolved;
	}

	return expanded;

err:
	free(resolved);
	free(expanded);
	return NULL;
}
