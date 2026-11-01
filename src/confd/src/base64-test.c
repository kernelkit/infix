/* SPDX-License-Identifier: BSD-3-Clause */
/* Unit test of base64_decode(), TAP output, run by 'make check' */

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static size_t alloc_size;

/* Fills every buffer base64.c allocates, so a missing NUL is not a lucky zero */
static void *poison_malloc(size_t size)
{
	void *ptr;

	ptr = malloc(size);
	if (ptr)
		memset(ptr, 0xa5, size);
	alloc_size = size;

	return ptr;
}

#define malloc poison_malloc
#include "base64.c"
#undef malloc

#define B64_63 "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFh"
#define TXT_63 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

struct test {
	const char *description;
	const char *input;
	const char *expected;	/* NULL: decoding fails */
	size_t      expected_len;
	int         expected_errno;	/* on failure, 0: not checked */
};

static const struct test tests[] = {
	/* positive */
	{ "9 bytes, no padding",        "cGFzc3dvcmQx", "password1", 9, 0 },
	{ "8 bytes, one pad",           "cGFzc3dvcmQ=", "password",  8, 0 },
	{ "7 bytes, two pads",          "cGFzc3dvcg==", "passwor",   7, 0 },
	{ "line feeds are skipped",     "cGFz\nc3dv\ncmQx\n", "password1", 9, 0 },
	{ "32 bytes binary, WireGuard key",
	  "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=",
	  "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f"
	  "\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f", 32, 0 },

	/* boundary */
	{ "one byte, the shortest output", "YQ==", "a", 1, 0 },
	{ "one block, three bytes",      "YWJj", "abc", 3, 0 },
	{ "8 bytes, shortest Wi-Fi passphrase", "MTIzNDU2Nzg=", "12345678", 8, 0 },
	{ "63 bytes, longest Wi-Fi passphrase", B64_63, TXT_63, 63, 0 },
	{ "highest byte values",         "////", "\xff\xff\xff", 3, 0 },

	/* negative */
	{ "empty input",                 "",      NULL, 0, EINVAL },
	{ "no base64 characters",        "!!!!",  NULL, 0, EINVAL },
	{ "three symbols",               "YWJ",   NULL, 0, EINVAL },
	{ "five symbols",                "YWJjZ", NULL, 0, EINVAL },
	{ "three pad characters",        "Y===",  NULL, 0, 0 },

	/* corner */
	{ "NUL in the output is counted", "YQBi", "a\0b", 3, 0 },
	{ "decoding stops after padding", "YQ==YWJj", "a", 1, 0 },
	{ "non-zero bits under two pads", "YR==", "a", 1, 0 },
	{ "non-zero bits under one pad", "YWJ=", "ab", 2, 0 },
};

/* Returns NULL when decoding t->input gives t->expected, else why not */
static const char *check(const struct test *t, int with_len)
{
	const char *why = NULL;
	unsigned char *out;
	size_t len = 0;
	int err;

	errno = 0;
	out = base64_decode((const unsigned char *)t->input, strlen(t->input),
			    with_len ? &len : NULL);
	err = errno;

	if (!t->expected) {
		if (out)
			why = "decoded invalid input";
		else if (t->expected_errno && err != t->expected_errno)
			why = "wrong errno";
	} else if (!out) {
		why = "failed to decode";
	} else if (with_len && len != t->expected_len) {
		why = "wrong length";
	} else if (alloc_size < t->expected_len) {
		why = "output too short";
	} else if (memcmp(out, t->expected, t->expected_len)) {
		why = "wrong output";
	} else if (alloc_size == t->expected_len) {
		why = "no room for a NUL terminator";
	} else if (out[t->expected_len] != '\0') {
		why = "not NUL terminated";
	}
	free(out);

	return why;
}

int main(void)
{
	size_t num = sizeof(tests) / sizeof(tests[0]);
	int failed = 0;

	printf("1..%zu\n", num * 2);
	for (size_t i = 0; i < num; i++) {
		for (int with_len = 1; with_len >= 0; with_len--) {
			const char *why = check(&tests[i], with_len);
			size_t n = 2 * i + 2 - with_len;

			if (why) {
				printf("not ok %zu - %s%s: %s\n", n, tests[i].description,
				       with_len ? "" : " (no out_len)", why);
				failed++;
			} else {
				printf("ok %zu - %s%s\n", n, tests[i].description,
				       with_len ? "" : " (no out_len)");
			}
		}
	}

	return failed ? 1 : 0;
}
