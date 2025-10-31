/* SPDX-License-Identifier: ISC */
#ifndef BIN_UTIL_H_
#define BIN_UTIL_H_
#include <stdbool.h>
#include <stdio.h>
#include <libite/lite.h>

#define ERRMSG "Error: "
#define INFMSG "Note: "

int         yorn       (const char *fmt, ...);

int         files      (const char *path, const char *stripext);

const char *basenm     (const char *fn);
int         has_ext    (const char *fn, const char *ext);
char       *cfg_adjust (const char *path, const char *template, bool sanitize);

#endif /* BIN_UTIL_H_ */
