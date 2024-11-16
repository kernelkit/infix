#include <ctype.h>
#include <dirent.h>
#include <errno.h>
#include <getopt.h>
#include <libgen.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#define SYSLOG_NAMES
#include <syslog.h>
#include <unistd.h>

#include <linux/netlink.h>
#include <linux/rtnetlink.h>

#include <sys/inotify.h>
#include <sys/socket.h>

#include <uev/uev.h>
#include <libite/lite.h>

#define err(fmt, args...)   syslog(LOG_ERR,     fmt ": %s", ##args, strerror(errno))
#define errx(fmt, args...)  syslog(LOG_ERR,     fmt, ##args)
#define warn(fmt, args...)  syslog(LOG_WARNING, fmt, ": %s", ##args, strerror(errno))
#define warnx(fmt, args...) syslog(LOG_WARNING, fmt, ##args)
#define log(fmt, args...)   syslog(LOG_NOTICE,  fmt, ##args)
#define dbg(fmt, args...)   syslog(LOG_DEBUG,   fmt, ##args)

/*
 * Allow SNN and KNN style jobs, for inotyify_cb() we also allow
 * a type '*' just to figure out if a job should be archived in
 * the done directory.
 */
static inline int should_run(const char *name, int type)
{
	if (!name || strlen(name) < 3)
		return 0;

	if (isdigit(name[1]) && isdigit(name[2])) {
		if (type == '*') {
			switch (name[0]) {
			case 'K':
			case 'S':
				return 1;
			default:
				goto done;
			}
		}

		switch (type) {
		case 'K':
		case 'S':
			break;
		default:
			return 0;
		}

		dbg("name:%s type:'%c' => run:%d", name, type, type == name[0]);
		return type == name[0];
	}
done:
	errx("unsupported script %s, must follow pattern SNN/KNN", name);
	return 0;
}

static inline int logmask_from_str(const char *str)
{
	const CODE *code;

	for (code = prioritynames; code->c_name; code++)
		if (!strcmp(str, code->c_name))
			return LOG_UPTO(code->c_val);

	return -1;
}

