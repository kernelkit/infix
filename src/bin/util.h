/* SPDX-License-Identifier: ISC */
#ifndef BIN_UTIL_H_
#define BIN_UTIL_H_
#include <stdbool.h>
#include <stdio.h>
#include <sys/stat.h>
#include <libite/lite.h>

#define CFG_DIR        "/cfg/"
#define STARTUP_CONFIG CFG_DIR "startup-config.cfg"

#define ERRMSG "Error: "
#define DBGMSG "Debug: "
#define INFMSG "Note: "

int         yorn       (const char *fmt, ...);

int         files      (const char *path, const char *stripext);

const char *basenm     (const char *fn);
int         dirlen     (const char *path);
int         has_ext    (const char *fn, const char *ext);
mode_t      path_mode  (const char *path);
bool        path_traversable(const char *path);
char       *cfg_adjust (const char *path, const char *template, bool sanitize);

#endif /* BIN_UTIL_H_ */
