/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef CONFD_SYSTEMV_H_
#define CONFD_SYSTEMV_H_

int systemv(char **args);
int systemv_silent(char **args);
int fsystemv(char **args, FILE *in, FILE *out, FILE *err);

#endif /* CONFD_SYSTEMV_H_ */
