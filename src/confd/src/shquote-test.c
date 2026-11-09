/* SPDX-License-Identifier: BSD-3-Clause */
/* Unit test of fputqw() and fputsq(), TAP output, run by 'make check' */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "shquote.c"

struct test {
	void (*fn)(const char *, FILE *);
	const char *description;
	const char *input;
	const char *expected;
};

static const struct test tests[] = {
	{ fputqw, "command with arguments",   "/usr/sbin/httpd -f -v -p 91",
	  " '/usr/sbin/httpd' '-f' '-v' '-p' '91'" },
	{ fputqw, "command substitution",     "$(reboot)",        " '$(reboot)'" },
	{ fputqw, "variable reference",       "sh -c echo ${HOME}",
	  " 'sh' '-c' 'echo' '${HOME}'" },
	{ fputqw, "single quote",             "it's",             " 'it'\\''s'" },
	{ fputqw, "surrounding blanks",       "  two \t spaces ", " 'two' 'spaces'" },
	{ fputqw, "empty",                    "",                 "" },
	{ fputsq, "one word with blanks",     "Uplink to core",   "'Uplink to core'" },
	{ fputsq, "comment and quotes",       "Port #3 \"lab\" it's", "'Port #3 \"lab\" it'\\''s'" },
	{ fputsq, "line break",               "a\nreboot",        "'a\nreboot'" },
	{ fputsq, "empty word",               "",                 "''" },
};

int main(void)
{
	size_t num = sizeof(tests) / sizeof(tests[0]);
	int failed = 0;

	printf("1..%zu\n", num);
	for (size_t i = 0; i < num; i++) {
		const struct test *t = &tests[i];
		char *out = NULL;
		size_t len = 0;
		FILE *fp;

		fp = open_memstream(&out, &len);
		t->fn(t->input, fp);
		fclose(fp);

		if (strcmp(out, t->expected)) {
			printf("not ok %zu - %s: got \"%s\"\n", i + 1, t->description, out);
			failed++;
		} else {
			printf("ok %zu - %s\n", i + 1, t->description);
		}
		free(out);
	}

	return failed ? 1 : 0;
}
