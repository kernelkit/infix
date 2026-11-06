/* SPDX-License-Identifier: BSD-3-Clause */
#include <stdio.h>
#include <string.h>

/*
 * Writes each word of str to fp, preceded by a space and single-quoted,
 * so the shell running the output passes the words on unexpanded.
 */
void fputqw(const char *str, FILE *fp)
{
	size_t len;

	for (; *str; str += len) {
		str += strspn(str, " \t");
		if (!*str)
			break;

		len = strcspn(str, " \t");
		fputs(" '", fp);
		for (size_t i = 0; i < len; i++) {
			if (str[i] == '\'')
				fputs("'\\''", fp);
			else
				fputc(str[i], fp);
		}
		fputc('\'', fp);
	}
}
