/* SPDX-License-Identifier: BSD-3-Clause */

#ifndef BASE64_H_
#define BASE64_H_

#include <stdlib.h>

unsigned char *base64_encode(const unsigned char *src, size_t len, size_t *out_len);
unsigned char *base64_decode(const unsigned char *src, size_t len, size_t *out_len);

#endif /* BASE64_H_ */
