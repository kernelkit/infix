/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HELPERS_H_
#define CONFD_HELPERS_H_

int runbg(char *const args[], int delay);
int run_status(int pid);

int fexistf(const char *fmt, ...)
	__attribute__ ((format (printf, 1, 2)));
FILE *popenf(const char *type, const char *cmdf, ...)
	__attribute__ ((format (printf, 2, 3)));

int vreadllf(long long *value, const char *fmt, va_list ap);
int readllf(long long *value, const char *fmt, ...)
	__attribute__ ((format (printf, 2, 3)));
int readdf(int *value, const char *fmt, ...)
	__attribute__ ((format (printf, 2, 3)));

int writedf(int value, const char *mode, const char *fmt, ...)
	__attribute__ ((format (printf, 3, 4)));
int writesf(const char *str, const char *mode, const char *fmt, ...)
	__attribute__ ((format (printf, 3, 4)));

#endif /* CONFD_HELPERS_H_ */
