/* SPDX-License-Identifier: MIT */

#include <getopt.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <linux/input.h>

static void usage()
{
	fputs("keyack - Wait for key press and release\n"
	      "\n"
	      "Usage:\n"
	      "  keyack [options]\n"
	      "\n"
	      "Options:\n"
	      "  -d DEVICE      Operate on DEVICE, instead of /dev/input/event0.\n"
	      "  -k KEYCODE     Wait for KEYCODE, instead of KEY_RESTART.\n"
	      "  -h             Print usage message and exit.\n",
	      stderr);
}

static const char *sopts = "d:hk:";
static struct option lopts[] = {
	{ "device", required_argument, 0, 'd' },
	{ "help",   no_argument,       0, 'h' },
	{ "key",    required_argument, 0, 'k' },

	{ NULL }
};

bool wait_key_state(FILE *fp, int code, int val)
{
	struct input_event ev;

	while (fread(&ev, sizeof(ev), 1, fp)) {
		if (ev.type == EV_KEY && ev.code == code && ev.value == val)
			return true;
        }

	return false;
}

int main(int argc, char **argv)
{
	const char *dev = "/dev/input/event0";
	int opt, code = KEY_RESTART;
	bool success;
	FILE *fp;

	while ((opt = getopt_long(argc, argv, sopts, lopts, NULL)) > 0) {
		switch (opt) {
		case 'd':
			dev = optarg;
			break;
		case 'h':
			usage(); exit(0);
			break;
		case 'k':
			code = strtol(optarg, NULL, 0);
			break;
		default:
			fprintf(stderr, "unknown option '%c'\n", opt);
			usage(); exit(1);
			break;
		}
	}

	fp = fopen(dev, "r");
	if (!fp) {
		fprintf(stderr, "Unable to open \"%s\": %m\n", dev);
		exit(1);
	}

	success = wait_key_state(fp, code, 1) && wait_key_state(fp, code, 0);

	fclose(fp);

	if (!success) {
		fprintf(stderr, "Failed to read events");
		exit(1);
	}

	return 0;
}
