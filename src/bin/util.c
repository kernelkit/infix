/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <dirent.h>
#include <errno.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <termios.h>

#include "util.h"

#define CFG_DIR "/cfg/"

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

/* Directories the CLI may access, mode and default extension of files there */
struct location {
	const char *prefix;
	mode_t      mode;
	const char *ext;
};

static const struct location allowed[] = {
	{ CFG_DIR,     0660, ".cfg" },
	{ "/media/",   0664, ""     },
	{ "/var/lib/", 0664, ""     },
	{ "/var/log/", 0664, ""     },
	{ "/log/",     0664, ""     },
	{ "/var/tmp/", 0664, ""     },
	{ "/tmp/",     0664, ""     },
};

/* Match a location, both the directory itself and anything below it */
static bool has_prefix(const char *path, const char *prefix)
{
	size_t len;

	if (!prefix)
		return false;

	len = strlen(prefix);
	if (!strncmp(path, prefix, len))
		return true;

	/* Trailing slash in the table, the path may be without */
	return len && prefix[len - 1] == '/' && !path[len - 1]
		&& !strncmp(path, prefix, len - 1);
}

static const struct location *path_lookup(const char *path)
{
	static struct location home = { NULL, 0660, "" };

	home.prefix = getenv("HOME");
	if (has_prefix(path, home.prefix))
		return &home;

	for (size_t i = 0; i < NELEMS(allowed); i++) {
		if (has_prefix(path, allowed[i].prefix))
			return &allowed[i];
	}

	return NULL;
}

mode_t path_mode(const char *path)
{
	const struct location *loc = path_lookup(path);

	return loc ? loc->mode : 0;
}

char *cfg_adjust(const char *path, const char *template, bool sanitize)
{
	char *expanded = NULL, *resolved = NULL, *full;
	const struct location *loc = NULL;
	const char *prefix = "";
	const char *basename;

	if (sanitize) {
		if (strstr(path, "../"))
			goto err;

		/* CLI users save to /cfg by default, unless abs. path */
		if (path[0] != '/')
			prefix = CFG_DIR;

		loc = path_lookup(*prefix ? prefix : path);
		if (!loc)
			goto err;
	}

	if (asprintf(&expanded, "%s%s", prefix, path) < 0)
		goto err;

	if (sanitize) {
		resolved = realpath(expanded, NULL);
		if (resolved) {
			/* Follow symlinks, the target must be allowed too */
			if (!path_mode(resolved))
				goto err;

			free(expanded);
			expanded = resolved;
			resolved = NULL;
		} else if (errno != ENOENT) {
			goto err;
		}
	}

	/* Directory destination, copy into it like cp(1) */
	if (template && fisdir(expanded)) {
		size_t len = strlen(expanded);

		basename = basenm(template);
		if (!basename)
			goto err;

		/* The path may already end in a slash, do not double it */
		while (len > 1 && expanded[len - 1] == '/')
			expanded[--len] = 0;

		if (asprintf(&full, "%s/%s", expanded, basename) < 0)
			goto err;

		free(expanded);
		expanded = full;
	}

	/* Config files get an extension, if the name lacks one */
	basename = basenm(expanded);
	if (loc && *loc->ext && basename && !strchr(basename, '.')) {
		if (asprintf(&full, "%s%s", expanded, loc->ext) < 0)
			goto err;

		free(expanded);
		expanded = full;
	}

	return expanded;

err:
	free(resolved);
	free(expanded);
	return NULL;
}
