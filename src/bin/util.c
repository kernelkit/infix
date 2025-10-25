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

const char *basenm(const char *fn)
{
	const char *ptr;

	if (!fn)
		return "";

	ptr = strrchr(fn, '/');
	if (ptr)
		ptr++;
	else
		ptr = fn;

	return ptr;
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

char *cfg_adjust(const char *fn, const char *tmpl, char *buf, size_t len, int sanitize)
{
	char tmp[256], resolved[PATH_MAX];

	if (strlen(basenm(fn)) == 0) {
		if (!tmpl)
			return NULL;

		fn = basenm(tmpl);
		/* Fall through */
	}

	if (sanitize) {
		if (strstr(fn, "../"))
			return NULL;

		if (fn[0] == '/') {
			if (!path_allowed(fn))
				return NULL;
		} else {
			snprintf(tmp, sizeof(tmp), "/cfg/%s", fn);
			if (!has_ext(tmp, ".cfg"))
				strlcat(tmp, ".cfg", sizeof(tmp));
			fn = tmp;
		}

		/* If file exists, resolve symlinks and verify still in whitelist */
		if (realpath(fn, resolved)) {
			if (!path_allowed(resolved))
				return NULL;
			fn = resolved;
		}
	}

	strlcpy(buf, fn, len);
	return buf;
}
