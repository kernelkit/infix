/* SPDX-License-Identifier: BSD-3-Clause */
#include <dirent.h>
#include <errno.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

#include <srx/common.h>

#include "base64.h"
#include "core.h"

#define SUPPORT_TOOL	"/usr/sbin/support"
#define SUPPORT_GPG	"/usr/bin/gpg"
#define SUPPORT_LOG_SEC 5

/* RAM, /var/lib is persistent storage on Infix */
#define SUPPORT_WORK	"/tmp"

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

/* The tool names the archive, we only know it is the one file that is
 * not the log a failed collection leaves behind */
static int archive(const char *dir, char *path, size_t len)
{
	struct dirent *d;
	int found = -1;
	DIR *dp;

	dp = opendir(dir);
	if (!dp)
		return -1;

	while ((d = readdir(dp))) {
		char *ext = strrchr(d->d_name, '.');

		if (d->d_name[0] == '.' || (ext && !strcmp(ext, ".log")))
			continue;

		snprintf(path, len, "%s/%s", dir, d->d_name);
		found = 0;
		break;
	}

	closedir(dp);
	return found;
}

static void cleanup(const char *dir)
{
	struct dirent *d;
	DIR *dp;

	dp = opendir(dir);
	if (dp) {
		while ((d = readdir(dp))) {
			if (!strcmp(d->d_name, ".") || !strcmp(d->d_name, ".."))
				continue;
			unlinkat(dirfd(dp), d->d_name, 0);
		}
		closedir(dp);
	}

	if (rmdir(dir))
		WARN("Cannot remove %s: %s", dir, strerror(errno));
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
	char dir[] = SUPPORT_WORK "/support-rpc-XXXXXX";
	char file[256], keep[256];
	const char *password = NULL;
	unsigned char *raw, *b64;
	const char *user, *name;
	struct stat st;
	size_t cnt = 0;
	FILE *pp;
	int rc;

	for (size_t i = 0; i < input_cnt; i++) {
		char *leaf = strrchr(input[i].xpath, '/');

		if (leaf && !strcmp(leaf, "/password") &&
		    input[i].data.string_val[0])
			password = input[i].data.string_val;
	}

	user = sr_session_get_user(session);
	AUDIT("Support data collection requested by user \"%s\".", user ?: "unknown");

	if (password && access(SUPPORT_GPG, X_OK)) {
		sr_session_set_netconf_error(session, "application", "operation-failed",
					     NULL, NULL, "gpg is not available on "
					     "this device", 0);
		return SR_ERR_OPERATION_FAILED;
	}

	if (!mkdtemp(dir)) {
		ERROR("Cannot create %s: %s", dir, strerror(errno));
		return SR_ERR_INTERNAL;
	}

	/* The password goes on stdin, never in the process list, which is
	 * also why the tool gets a directory of its own instead of us
	 * reading the archive name off its stdout */
	pp = popenf("w", SUPPORT_TOOL " --work-dir %s collect --log-sec %u -o %s%s",
		    dir, SUPPORT_LOG_SEC, dir, password ? " -p" : "");
	if (!pp) {
		ERROR("Failed running %s: %s", SUPPORT_TOOL, strerror(errno));
		cleanup(dir);
		return SR_ERR_INTERNAL;
	}

	if (password)
		fprintf(pp, "%s\n", password);

	rc = pclose(pp);
	if (rc) {
		ERROR("Support data collection failed, exit code %d", rc);
		return fail(session, output, cnt, "Support data collection failed", dir);
	}

	if (archive(dir, file, sizeof(file)) || stat(file, &st)) {
		ERROR("No support archive in %s: %s", dir, strerror(errno));
		return fail(session, output, cnt, "Support archive is missing", dir);
	}

	if (add_uint32(output, &cnt, path, "size", st.st_size))
		return fail(session, output, cnt, "Out of memory", dir);

	if (st.st_size > SUPPORT_LIMIT) {
		NOTE("Support archive %s is %jd bytes, too large to return inline.",
		     file, (intmax_t)st.st_size);

		/* Out of the directory we are about to remove */
		name = strrchr(file, '/');
		snprintf(keep, sizeof(keep), "%s/%s", SUPPORT_WORK,
			 name ? name + 1 : file);
		if (rename(file, keep))
			return fail(session, output, cnt,
				    "Cannot keep support archive", dir);
		cleanup(dir);

		if (add_str(output, &cnt, path, "filename", SR_STRING_T, keep))
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
