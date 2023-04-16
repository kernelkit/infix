/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdarg.h>
#include "core.h"

/*
 * Write interger value to a file composed from fmt and optional args.
 */
int writedf(int value, const char *fmt, ...)
{
	va_list ap;
	int rc = -1;
	FILE *fp;

	va_start(ap, fmt);
	fp = fopenf("w", fmt, ap);
	if (fp) {
		fprintf(fp, "%d\n", value);
		rc = fclose(fp);
	}
	va_end(ap);

	return rc;
}

/*
 * Write str to a file composed from fmt and optional args.
 */
int writesf(const char *str, const char *fmt, ...)
{
	va_list ap;
	int rc = -1;
	FILE *fp;

	va_start(ap, fmt);
	fp = fopenf("w", fmt, ap);
	if (fp) {
		fprintf(fp, "%s\n", str);
		rc = fclose(fp);
	}
	va_end(ap);

	return rc;
}

