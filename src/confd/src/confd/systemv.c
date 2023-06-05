/* SPDX-License-Identifier: BSD-3-Clause */

#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <string.h>		/* strerror() */
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
#include <stdlib.h>

/**
 * Reimplementation of system() without /bin/sh intermediary
 */
static int fsystemv(char **args, FILE *in, FILE *out, FILE *err)
{
	struct sigaction sa = { .sa_handler = SIG_IGN };
	sigset_t oldmask;
	int rc = -1;
	pid_t pid;

	if (!args) {
		errno = EINVAL;
		return -1;
	}

	/* Wait for last child to terminate, see waitpid(2) */
	sigaddset(&sa.sa_mask, SIGCHLD);
	sigprocmask(SIG_BLOCK, &sa.sa_mask, &oldmask);

	pid = fork();
	if (0 == pid) {
		sigprocmask(SIG_SETMASK, &oldmask, NULL);

		if (in)
			dup2(fileno(in), STDIN_FILENO);
		if (out)
			dup2(fileno(out), STDOUT_FILENO);
		if (err)
			dup2(fileno(err), STDERR_FILENO);

		_exit(execvp(args[0], args));
	}
	if (pid == -1 || waitpid(pid, &rc, 0) == -1)
		goto fail;

	if (WIFEXITED(rc)) {
		errno = 0;
		rc = WEXITSTATUS(rc);
	} else if (WIFSIGNALED(rc)) {
		errno = EINTR;
		rc = -1;
	}
fail:
	sigprocmask(SIG_SETMASK, &oldmask, NULL);
	return rc;
}

int systemv(char **args)
{
	return fsystemv(args, NULL, NULL, NULL);
}

int systemv_silent(char **args)
{
	FILE *out = fopen("/dev/null", "w");
	int rc;

	rc = fsystemv(args, NULL, out, out);
	if (out)
		fclose(out);

	return rc;
}

#ifdef UNITTEST
int main(void)
{
	char *args[] = {
		"ls", "/etc", NULL
	};
	int rc;

	rc = systemv_silent(args);
	printf("=> %d\n", rc);
}
#endif
