/* SPDX-License-Identifier: BSD-3-Clause */

#include "core.h"

/**
 * Like system(), but takes a formatted string as argument.
 * @param fmt  printf style format list to command to run
 *
 * This system() wrapper greatly simplifies operations that usually
 * consist of composing a command from parts into a dynamic buffer
 * before calling it.  The return value from system() is also parsed,
 * checking for proper exit and signals.
 *
 * @returns If the command exits normally, the return code of the command
 * is returned.  Otherwise, if the command is signalled, the return code
 * is -1 and @a errno is set to @c EINTR.
 */
int run(const char *fmt, ...)
{
	va_list ap;
	char *cmd;
	int len;
	int rc;

	va_start(ap, fmt);
	len = vsnprintf(NULL, 0, fmt, ap);
	va_end(ap);

	cmd = alloca(++len);
	if (!cmd) {
		errno = ENOMEM;
		return -1;
	}

	va_start(ap, fmt);
	vsnprintf(cmd, len, fmt, ap);
	va_end(ap);

	rc = system(cmd);
	if (rc == -1)
		return -1;

	if (WIFEXITED(rc)) {
		errno = 0;
		rc = WEXITSTATUS(rc);
	} else if (WIFSIGNALED(rc)) {
		errno = EINTR;
		rc = -1;
	}

	return rc;
}
