/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_HELPERS_H_
#define CONFD_HELPERS_H_

FILE *popenf(const char *type, const char *cmdf, ...);

int writedf(int value, const char *fmt, ...);
int writesf(const char *str, const char *fmt, ...);

#endif /* CONFD_HELPERS_H_ */
