/* SPDX-License-Identifier: BSD-3-Clause */
/*
 * XXX: Helper functions not yet available in a libite (-lite) release.
 * XXX: With the next major release, v2.6.0, these will clash and can be
 * XXX: removed.
 */

#include <errno.h>
#include <stdarg.h>
#include <stdio.h>
#include <limits.h>
#include <stdlib.h>
#include <sys/wait.h>
#include <libite/lite.h>

int debug;			/* Sets debug level (0:off) */

/* TODO remove once confd / statd lib situation is resolved */
#ifndef vasprintf
int vasprintf(char **strp, const char *fmt, va_list ap);
#endif

char *unquote(char *buf)
{
	char q = buf[0];
	char *ptr;

	if (q != '"' && q != '\'')
		return buf;

	ptr = &buf[strlen(buf) - 1];
	if (*ptr == q) {
		*ptr = 0;
		buf++;
	}

	return buf;
}

/*
 * Reads file, line by line, lookging for key="val".
 * Returns val, or NULL.
 */
char *fgetkey(const char *file, const char *key)
{
	static char line[256];
	int len = strlen(key);
	char *ptr = NULL;
	FILE *fp;

	fp = fopen(file, "r");
	if (!fp)
		return NULL;

	while (fgets(line, sizeof(line), fp)) {
		chomp(line);
		if (strncmp(line, key, len))
			continue;
		if (line[len] != '=')
			continue;

		ptr = unquote(line + len + 1); /* Skip 'key=' */
		break;
	}
	fclose(fp);

	return ptr;
}
