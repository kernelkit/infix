/* SPDX-License-Identifier: BSD-3-Clause */
/* Unit test of fputqw(), TAP output, run by 'make check' */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "shquote.c"

struct test {
	const char *description;
	const char *input;
	const char *expected;
};

static const struct test tests[] = {
	{ "command with arguments",   "/usr/sbin/httpd -f -v -p 91",
	  " '/usr/sbin/httpd' '-f' '-v' '-p' '91'" },
	{ "command substitution",     "$(reboot)",        " '$(reboot)'" },
	{ "variable reference",       "sh -c echo ${HOME}",
	  " 'sh' '-c' 'echo' '${HOME}'" },
	{ "single quote",             "it's",             " 'it'\\''s'" },
	{ "surrounding blanks",       "  two \t spaces ", " 'two' 'spaces'" },
	{ "empty",                    "",                 "" },
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
		fputqw(t->input, fp);
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
