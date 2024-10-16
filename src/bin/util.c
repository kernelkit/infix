/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <dirent.h>
#include <errno.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>
#include <termios.h>

#include "util.h"


static char rawgetch(void)
{
	struct termios saved, c;
	char key;

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

	return key;
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

static const char *basenm(const char *fn)
{
	const char *ptr;

	if (!fn)
		return "";

	ptr = strrchr(fn, '/');
	if (!ptr)
		ptr = fn;

	return ptr;
}

char *cfg_adjust(const char *fn, const char *tmpl, char *buf, size_t len)
{
	if (strstr(fn, "../"))
		return NULL;	/* relative paths not allowed */

	if (fn[0] == '/') {
		strlcpy(buf, fn, len);
		return buf;	/* allow absolute paths */
	}

	/* Files in /cfg must end in .cfg */
	if (!strncmp(fn, "/cfg/", 5)) {
		strlcpy(buf, fn, len);
		if (!has_ext(fn, ".cfg"))
			strlcat(buf, ".cfg", len);

		return buf;
	}

	/* Files ending with .cfg belong in /cfg */
	if (has_ext(fn, ".cfg")) {
		snprintf(buf, len, "/cfg/%s", fn);
		return buf;
	}

	if (strlen(fn) > 0 && fn[0] == '.' && tmpl) {
		if (fn[1] == '/' && fn[2] != 0)
			strlcpy(buf, fn, len);
		else
			strlcpy(buf, basenm(tmpl), len);
	} else
		strlcpy(buf, fn, len);

	return buf;
}
