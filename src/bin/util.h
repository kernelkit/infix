/* SPDX-License-Identifier: ISC */
#ifndef BIN_UTIL_H_
#define BIN_UTIL_H_
#include <stdio.h>
#include <libite/lite.h>

#define ERRMSG "Error: "
#define INFMSG "Note: "

int   yorn       (const char *fmt, ...);

int   files      (const char *path, const char *stripext);

int   has_ext    (const char *fn, const char *ext);
char *cfg_adjust (const char *fn, const char *tmpl, char *buf, size_t len);

#endif /* BIN_UTIL_H_ */
