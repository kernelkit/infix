/* SPDX-License-Identifier: BSD-3-Clause */
#include <dirent.h>
#include <errno.h>
#include <ftw.h>
#include <limits.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>

#include <srx/common.h>

#include "base64.h"
#include "core.h"

#define SUPPORT_TOOL	"/usr/sbin/support"
#define SUPPORT_GPG	"/usr/bin/gpg"
#define SUPPORT_LOG_SEC 5

/* Where the support command collects too, so 'support clean' covers
 * what is left behind.  RAM when the persistent storage is unusable. */
#define SUPPORT_DIR	"/var/lib/support"
#define SUPPORT_TMP	"/tmp"

/* Held in RAM raw, base64 encoded, in sysrepo, and in netopeer2 or
 * rousette.  Bigger archives are left for out-of-band fetching. */
#define SUPPORT_LIMIT	(16 * 1024 * 1024)

static unsigned char *slurp(const char *fn, size_t len)
{
	unsigned char *buf;
	FILE *fp;

	buf = malloc(len);
	if (!buf)
		return NULL;

	fp = fopen(fn, "r");
	if (!fp) {
		free(buf);
		return NULL;
	}

	if (fread(buf, 1, len, fp) != len) {
		fclose(fp);
		free(buf);
		return NULL;
	}

	fclose(fp);
	return buf;
}

/* libyang rejects the line feeds base64_encode() wraps with */
static void strip_lf(unsigned char *str)
{
	unsigned char *src = str, *dst = str;

	while (*src) {
		if (*src != '\n')
			*dst++ = *src;
		src++;
	}

	*dst = 0;
}

/* Same mode as the tool gives it, our umask is stricter */
static const char *workdir(void)
{
	if (!mkdir(SUPPORT_DIR, 0755))
		chmod(SUPPORT_DIR, 0755);
	else if (errno != EEXIST)
		return SUPPORT_TMP;

	if (access(SUPPORT_DIR, W_OK))
		return SUPPORT_TMP;

	return SUPPORT_DIR;
}

/* The tool names both the archive and the log of a failed run, the
 * extension is what tells them apart */
static int find_file(const char *dir, int log, char *path, size_t len)
{
	struct dirent *d;
	int found = -1;
	DIR *dp;

	dp = opendir(dir);
	if (!dp)
		return -1;

	while ((d = readdir(dp))) {
		char *ext = strrchr(d->d_name, '.');
		int is_log = ext && !strcmp(ext, ".log");

		if (d->d_name[0] == '.' || is_log != log)
			continue;

		snprintf(path, len, "%s/%s", dir, d->d_name);
		found = 0;
		break;
	}

	closedir(dp);
	return found;
}

/* Move out of the private directory, under the canonical name the
 * tool gave it, before the directory is removed.  rename() replaces a
 * symlink squatting the name rather than following it. */
static int keep(const char *dir, char *path, size_t len)
{
	const char *name = strrchr(path, '/');
	const char *slash = strrchr(dir, '/');
	char dst[PATH_MAX];

	if (!name || !slash)
		return -1;

	snprintf(dst, sizeof(dst), "%.*s%s", (int)(slash - dir), dir, name);
	if (rename(path, dst))
		return -1;

	snprintf(path, len, "%s", dst);
	return 0;
}

/* The tool says on stderr why it gave up, on an "Error:" line.  The
 * rest is noise, and a full filesystem truncates it. */
static void reason(const char *dir, char *buf, size_t len)
{
	char line[256], path[PATH_MAX];
	FILE *fp;

	buf[0] = 0;
	snprintf(path, sizeof(path), "%s/.stderr", dir);
	fp = fopen(path, "r");
	if (!fp)
		return;

	while (fgets(line, sizeof(line), fp)) {
		line[strcspn(line, "\n")] = 0;
		if (!strncmp(line, "Error: ", 7))
			snprintf(buf, len, "%s", line + 7);
	}

	fclose(fp);
}

static int rm_cb(const char *path, const struct stat *st, int flag, struct FTW *ftw)
{
	(void)st;
	(void)ftw;

	if (flag == FTW_DP || flag == FTW_D)
		return rmdir(path);

	return unlink(path);
}

/* A tool killed mid-run leaves its collection directory behind */
static void cleanup(const char *dir)
{
	if (nftw(dir, rm_cb, 16, FTW_DEPTH | FTW_PHYS))
		WARN("Cannot remove %s: %s", dir, strerror(errno));
}

/* The event session runs as confd itself, the caller is only known
 * from the originator data: netopeer2 pushes the NETCONF session id
 * and then the username, rousette pushes nothing */
static const char *rpc_user(sr_session_ctx_t *session, const char **via)
{
	const char *orig = sr_session_get_orig_name(session);
	const void *data;
	uint32_t size;

	*via = orig && orig[0] ? orig : "local session";

	if (orig && !strcmp(orig, "netopeer2") &&
	    !sr_session_get_orig_data(session, 1, &size, &data) && size)
		return data;

	return NULL;
}

static int add_str(sr_val_t **output, size_t *cnt, const char *path,
		   const char *leaf, sr_val_type_t type, const char *val)
{
	if (sr_realloc_values(*cnt, *cnt + 1, output))
		return -1;

	/* Count it now, the caller frees *cnt values on failure */
	(*cnt)++;

	if (sr_val_build_xpath(&(*output)[*cnt - 1], "%s/%s", path, leaf))
		return -1;

	return sr_val_set_str_data(&(*output)[*cnt - 1], type, val) ? -1 : 0;
}

static int add_uint32(sr_val_t **output, size_t *cnt, const char *path,
		      const char *leaf, uint32_t val)
{
	if (sr_realloc_values(*cnt, *cnt + 1, output))
		return -1;

	(*cnt)++;

	if (sr_val_build_xpath(&(*output)[*cnt - 1], "%s/%s", path, leaf))
		return -1;

	(*output)[*cnt - 1].type = SR_UINT32_T;
	(*output)[*cnt - 1].data.uint32_val = val;

	return 0;
}

/* Drop the archive, the caller gets no filename and cannot clean up */
static int fail(sr_session_ctx_t *session, sr_val_t **output, size_t cnt,
		const char *msg, const char *dir)
{
	sr_free_values(*output, cnt);
	*output = NULL;

	if (dir)
		cleanup(dir);

	sr_session_set_netconf_error(session, "application", "operation-failed",
				     NULL, NULL, msg, 0);
	return SR_ERR_OPERATION_FAILED;
}

static int rpc_collect(sr_session_ctx_t *session, uint32_t sub_id, const char *path,
		       const sr_val_t *input, const size_t input_cnt, sr_event_t event,
		       unsigned request_id, sr_val_t **output, size_t *output_cnt,
		       void *priv)
{
	char dir[PATH_MAX], file[PATH_MAX], msg[PATH_MAX + 256], why[200];
	const char *password = NULL;
	const char *user, *via, *work;
	unsigned char *raw, *b64;
	struct stat st;
	size_t cnt = 0;
	FILE *pp;
	int rc;

	/* Abort follows a successful callback the originator stopped
	 * waiting for, nothing to undo but the archive is gone */
	if (event != SR_EV_RPC) {
		NOTE("Support data collection outlived the RPC timeout, archive discarded.");
		return SR_ERR_OK;
	}

	for (size_t i = 0; i < input_cnt; i++) {
		char *leaf = strrchr(input[i].xpath, '/');

		if (leaf && !strcmp(leaf, "/password") &&
		    input[i].data.string_val[0])
			password = input[i].data.string_val;
	}

	user = rpc_user(session, &via);
	AUDIT("Support data collection requested by user \"%s\" over %s.",
	      user ?: "unknown", via);

	/* The tool reads it as one line */
	if (password && strpbrk(password, "\r\n")) {
		sr_session_set_netconf_error(session, "application", "invalid-value",
					     NULL, NULL, "password must be a single "
					     "line", 0);
		return SR_ERR_INVAL_ARG;
	}

	if (password && access(SUPPORT_GPG, X_OK)) {
		sr_session_set_netconf_error(session, "application", "operation-failed",
					     NULL, NULL, "gpg is not available on "
					     "this device", 0);
		return SR_ERR_OPERATION_FAILED;
	}

	work = workdir();

	/* What earlier calls left behind, the caller cannot clean up */
	systemf(SUPPORT_TOOL " --work-dir %s clean >/dev/null 2>&1", work);

	snprintf(dir, sizeof(dir), "%s/support-rpc-XXXXXX", work);
	if (!mkdtemp(dir)) {
		ERROR("Cannot create %s: %s", dir, strerror(errno));
		return SR_ERR_INTERNAL;
	}

	/* The password goes on stdin, never in the process list, which is
	 * also why the tool gets a directory of its own instead of us
	 * reading the archive name off its stdout */
	pp = popenf("w", SUPPORT_TOOL " --work-dir %s collect --log-sec %u -o %s%s 2>%s/.stderr",
		    dir, SUPPORT_LOG_SEC, dir, password ? " -p" : "", dir);
	if (!pp) {
		ERROR("Failed running %s: %s", SUPPORT_TOOL, strerror(errno));
		cleanup(dir);
		return SR_ERR_INTERNAL;
	}

	if (password)
		fprintf(pp, "%s\n", password);

	rc = pclose(pp);
	if (rc == -1 || !WIFEXITED(rc) || WEXITSTATUS(rc)) {
		if (rc != -1 && WIFEXITED(rc))
			ERROR("Support data collection failed, exit code %d", WEXITSTATUS(rc));
		else
			ERROR("Support data collection failed: %s", rc == -1 ? strerror(errno) : "killed");

		reason(dir, why, sizeof(why));
		if (!find_file(dir, 1, file, sizeof(file)) && !keep(dir, file, sizeof(file)))
			snprintf(msg, sizeof(msg), "Support data collection failed%s%s, see %s",
				 why[0] ? ": " : "", why, file);
		else
			snprintf(msg, sizeof(msg), "Support data collection failed%s%s",
				 why[0] ? ": " : "", why);

		return fail(session, output, cnt, msg, dir);
	}

	if (find_file(dir, 0, file, sizeof(file)) || stat(file, &st)) {
		ERROR("No support archive in %s: %s", dir, strerror(errno));
		return fail(session, output, cnt, "Support archive is missing", dir);
	}

	if (add_uint32(output, &cnt, path, "size", st.st_size))
		return fail(session, output, cnt, "Out of memory", dir);

	if (st.st_size > SUPPORT_LIMIT) {
		NOTE("Support archive %s is %jd bytes, too large to return inline.",
		     file, (intmax_t)st.st_size);

		if (keep(dir, file, sizeof(file)))
			return fail(session, output, cnt, "Cannot keep support archive", dir);
		cleanup(dir);

		if (add_str(output, &cnt, path, "filename", SR_STRING_T, file))
			return fail(session, output, cnt, "Out of memory", NULL);

		*output_cnt = cnt;
		return SR_ERR_OK;
	}

	raw = slurp(file, st.st_size);
	if (!raw) {
		ERROR("Cannot read support archive %s: %s", file, strerror(errno));
		return fail(session, output, cnt, "Cannot read support archive", dir);
	}

	b64 = base64_encode(raw, st.st_size, NULL);
	free(raw);
	if (!b64)
		return fail(session, output, cnt, "Cannot encode support archive", dir);

	strip_lf(b64);
	rc = add_str(output, &cnt, path, "data", SR_BINARY_T, (char *)b64);
	free(b64);
	if (rc)
		return fail(session, output, cnt, "Out of memory", dir);

	/* The caller has it now */
	cleanup(dir);

	*output_cnt = cnt;
	return SR_ERR_OK;
}

int support_rpc_init(struct confd *confd)
{
	int rc = 0;

	REGISTER_RPC(confd->session, "/infix-system:support-collect",
		     rpc_collect, NULL, &confd->sub);
fail:
	return rc;
}
