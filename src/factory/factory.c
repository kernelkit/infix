#include <err.h>
#include <errno.h>
#include <fcntl.h>
#include <getopt.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <termios.h>
#include <unistd.h>
#include <sys/stat.h>
#include <sys/types.h>

#define RESETME    "/mnt/cfg/infix/.reset"
#define touch(f)   mknod((f), S_IFREG|0644, 0)

int rawgetch(void)
{
	struct termios saved, c;
	int key;

	if (tcgetattr(fileno(stdin), &saved) < 0)
		return -1;

	c = saved;
	c.c_lflag &= ~ICANON;
	c.c_lflag &= ~ECHO;
	c.c_cc[VMIN] = 1;
	c.c_cc[VTIME] = 0;

	if (tcsetattr(fileno(stdin), TCSANOW, &c) < 0) {
		tcsetattr(fileno(stdin), TCSANOW, &saved);
		return -1;
	}

	key = getchar();
	tcsetattr(fileno(stdin), TCSANOW, &saved);

	return key;
}

int yorn(const char *prompt)
{
	char yorn;

	fputs(prompt, stderr);

	yorn = rawgetch();
	fprintf(stderr, "%c\n", yorn);
	if (yorn != 'y' && yorn != 'Y')
		return 0;

	return 1;
}

static int usage(int rc)
{
	printf("usage: factory [opts]\n"
	       "\n"
	       "Options:\n"
	       " -h, --help        This help text\n"
	       " -r, --no-reboot   Don't reboot.  Reboot manually to activate.\n"
	       " -y, --assume-yes  Automatic yes to prompts; assume \"yes\" as answer to all\n"
	       "                   prompts and run non-interactively\n"
	       "\n"
	       "Note: this program initiates a factory reset by raising a flag and rebooting\n"
	       "      the system.  When it comes back up it safely removes all the OverlayFS\n"
	       "      worker/upper directories for /etc, /home, and /var\n");

	return rc;
}

static int run(const char *cmd)
{
	int status = system(cmd);
	int rc;

	rc = WEXITSTATUS(status);

	if (WIFEXITED(status))
		return rc;

	if (WIFSIGNALED(status)) {
		if (rc == 0)
			rc = 1;	/* adjust, we were aborted */
	}

	return rc;
}

int main(int argc, char *argv[])
{
	struct option long_opts[] = {
		{ "help",       0, NULL, 'h' },
		{ "no-reboot",  0, NULL, 'r' },
		{ "assume-yes", 0, NULL, 'y' },
		{ NULL, 0, NULL, 0 }
	};
	int reboot = 1;
	int yes = 0;
	char *tty;
	int c;

	while ((c = getopt_long(argc, argv, "h?ry", long_opts, NULL)) != EOF) {
		switch (c) {
		case 'h':
		case '?':
			return usage(0);

		case 'r':
			reboot = 0;
			break;

		case 'y':
			yes = 1;
			break;

		default:
			return usage(1);
		}
	}

	tty = ttyname(STDIN_FILENO);
	if (!tty && errno == ENOTTY)
		yes    = 1;

	if (argv[0][0] == '-' && tty && strcmp(tty, "/dev/console"))
		errx(1, "factory reset only allowed from console login!");

	if (yes || yorn("Factory reset device (y/N)? ")) {
		if (touch(RESETME) && errno != EEXIST)
			err(1, "failed");

		warnx("scheduled factory reset on next boot.");
		if (reboot && (yes || yorn("Reboot now to perform reset, (y/N)? ")))
			return run("/sbin/reboot");

		warnx("remember to reboot the system to perform the factory reset.");
	}

	return 0;
}

/**
 * Local Variables:
 *  indent-tabs-mode: t
 *  c-file-style: "linux"
 * End:
 */
