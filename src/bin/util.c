/* SPDX-License-Identifier: ISC */
#include "config.h"

#include <dirent.h>
#include <errno.h>
#include <limits.h>
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
	{ "/var/log/", 0660, ""     },
	{ "/log/",     0660, ""     },
	{ "/var/tmp/", 0660, ""     },
	{ "/tmp/",     0660, ""     },
};

/*
 * True when path is dir itself, or something below it.  The match ends
 * on a path component, so /home/jock does not cover /home/jocke.
 */
static bool path_within(const char *path, const char *dir)
{
	size_t len;

	if (!path || !dir)
		return false;

	len = strlen(dir);
	while (len > 1 && dir[len - 1] == '/')
		len--;

	if (strncmp(path, dir, len))
		return false;

	/* Root is above every path, others end on a component */
	if (len == 1 && dir[0] == '/')
		return path[0] == '/';

	return !path[len] || path[len] == '/';
}

static const struct location *path_lookup(const char *path)
{
	static struct location home = { NULL, 0660, "" };

	home.prefix = getenv("HOME");
	if (path_within(path, home.prefix))
		return &home;

	for (size_t i = 0; i < NELEMS(allowed); i++) {
		if (path_within(path, allowed[i].prefix))
			return &allowed[i];
	}

	return NULL;
}

/*
 * True for a path inside an allowed location, and for the directories
 * on the way to one, so a path can be walked to reach them.
 */
bool path_traversable(const char *path)
{
	const char *home = getenv("HOME");

	if (path_mode(path))
		return true;

	/* On the way to a location, e.g. /var leading to /var/lib */
	if (path_within(home, path))
		return true;

	for (size_t i = 0; i < NELEMS(allowed); i++) {
		if (path_within(allowed[i].prefix, path))
			return true;
	}

	return false;
}

mode_t path_mode(const char *path)
{
	const struct location *loc = path_lookup(path);

	return loc ? loc->mode : 0;
}

/*
 * A path that does not exist cannot be resolved, so the closest
 * existing parent decides.  Only it can be a symlink leading out
 * of an allowed location.
 */
static bool parent_allowed(const char *path)
{
	char dir[PATH_MAX];
	char *resolved;
	bool ok;
	int len;

	for (len = dirlen(path); len > 0; len = dirlen(dir)) {
		if ((size_t)len >= sizeof(dir))
			return false;

		strlcpy(dir, path, (size_t)len + 1);
		resolved = realpath(dir, NULL);
		if (resolved) {
			ok = path_mode(resolved) != 0;
			free(resolved);

			return ok;
		}

		if (errno != ENOENT)
			return false;
	}

	return false;
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
		} else if (errno == ENOENT) {
			if (!parent_allowed(expanded))
				goto err;
		} else {
			goto err;
		}
	}

	/* Directory destination, copy into it like cp(1).  A trailing
	 * slash says directory even when it is not there yet.
	 */
	if (template && (fisdir(expanded) || expanded[strlen(expanded) - 1] == '/')) {
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
