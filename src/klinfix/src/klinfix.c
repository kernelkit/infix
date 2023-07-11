#include <assert.h>
#include <dirent.h>
#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>

#include <sysrepo.h>
#include <sysrepo_types.h>
#include <sysrepo/values.h>

#include <klish/kplugin.h>
#include <klish/kcontext.h>

#include <libyang/libyang.h>

const uint8_t kplugin_klinfix_major = 1;
const uint8_t kplugin_klinfix_minor = 0;

int klix_files(kcontext_t *ctx)
{
	const char *path;
	struct dirent *d;
	DIR *dir;

	path = kcontext_script(ctx);
	if (!path) {
		fprintf(stderr, "Error: missing path argument to file search.\n");
		return -1;
	}

	dir = opendir(path);
	if (!dir) {
		fprintf(stderr, "Error: %s", strerror(errno));
		return -1;
	}

	while ((d = readdir(dir))) {
		if (d->d_type != DT_REG)
			continue;

		printf("%s\n", d->d_name);
	}
	closedir(dir);

	return 0;
}

int klix_ds_from_str(const char *text, sr_datastore_t *ds)
{
	size_t len = strlen(text);

	if (!strncmp("startup-config", text, len))
		*ds = SR_DS_STARTUP;
	else if (!strncmp("running-config", text, len))
		*ds = SR_DS_RUNNING;
	else if (!strncmp("candidate-config", text, len))
		*ds = SR_DS_CANDIDATE;
	else if (!strncmp("operational-config", text, len))
		*ds = SR_DS_OPERATIONAL;
	else if (!strncmp("factory-config", text, len))
		*ds = SR_DS_FACTORY_DEFAULT;
	else
		return -1;

	return 0;
}

int klix_copy(kcontext_t *ctx)
{
	kpargv_t *pargv = kcontext_pargv(ctx);
	sr_datastore_t srcds, dstds;
	kparg_t *srcarg, *dstarg;
	sr_session_ctx_t *sess;
	sr_conn_ctx_t *conn;
	int err;

	srcarg = kpargv_find(pargv, "src");
	dstarg = kpargv_find(pargv, "dst");
	if (!srcarg || !dstarg)
		goto err;

	if (klix_ds_from_str(kparg_value(srcarg), &srcds)) {
		fprintf(stderr,
			"Error: \"%s\" is not the name of any known datastore\n",
			kparg_value(srcarg));
		goto err;
	}
	if (klix_ds_from_str(kparg_value(dstarg), &dstds)) {
		fprintf(stderr,
			"Error: \"%s\" is not the name of any known datastore\n",
			kparg_value(dstarg));
		goto err;
	}

	switch (srcds) {
	case SR_DS_STARTUP:
	case SR_DS_RUNNING:
	case SR_DS_FACTORY_DEFAULT:
		break;
	default:
		fprintf(stderr,
			"Error: \"%s\" is not a valid source datastore\n",
			kparg_value(srcarg));
		goto err;
	}

	switch (dstds) {
	case SR_DS_STARTUP:
	case SR_DS_RUNNING:
		break;
	default:
		fprintf(stderr,
			"Error: \"%s\" is not a valid destination datastore\n",
			kparg_value(dstarg));
		goto err;
	}

	if (sr_connect(SR_CONN_DEFAULT, &conn)) {
		fprintf(stderr, "Error: Connection to datastore failed\n");
		goto err;
	}
	if (sr_session_start(conn, dstds, &sess)) {
		fprintf(stderr, "Error: Unable to open transaction to %s\n",
			kparg_value(dstarg));
		goto err_disconnect;
	}

	err = sr_copy_config(sess, NULL, srcds, 0);
	if (err) {
		fprintf(stderr, "Error: Unable to copy configuration (%d)\n",
			err);
		goto err_disconnect;
	}

	sr_disconnect(conn);
	return 0;

err_disconnect:
	sr_disconnect(conn);
err:
	return -1;
}

int klix_commit(kcontext_t *ctx)
{
	sr_session_ctx_t *sess;
	sr_conn_ctx_t *conn;
	int err;

	if (sr_connect(SR_CONN_DEFAULT, &conn)) {
		fprintf(stderr, "Error: Connection to datastore failed\n");
		goto err;
	}

	if (sr_session_start(conn, SR_DS_RUNNING, &sess)) {
		fprintf(stderr,
			"Error: Unable to open transaction to running-config\n");
		goto err_disconnect;
	}

	err = sr_copy_config(sess, NULL, SR_DS_CANDIDATE, 0);
	if (err) {
		fprintf(stderr,
			"Error: Unable to commit candidate to running (%d)\n",
			err);
		goto err;
	}

	sr_disconnect(conn);
	return 0;

err_disconnect:
	sr_disconnect(conn);
err:
	return -1;
}

int klix_rpc(kcontext_t *ctx)
{
	kpargv_pargs_node_t *iter;
	size_t icnt = 0, ocnt = 0;
	sr_val_t *input = NULL;
	sr_session_ctx_t *sess;
	sr_conn_ctx_t *conn;
	const char *xpath;
	sr_val_t *output;
	kparg_t *parg;
	int err;

	xpath = kcontext_script(ctx);
	if (!xpath) {
		fprintf(stderr, "Error: cannot find rpc xpath\n");
		goto err;
	}

	iter = kpargv_pargs_iter(kcontext_pargv(ctx));
	while ((parg = kpargv_pargs_each(&iter))) {
		const char *key = kentry_name(kparg_entry(parg));
		const char *val = kparg_value(parg);

		/* skip leading part of command line: 'set datetime' */
		if (!strcmp(key, val))
			continue;

		sr_realloc_values(icnt, icnt + 1, &input);
		/* e.g. /ietf-system:set-current-datetime/current-datetime */
		sr_val_build_xpath(&input[icnt], "%s/%s", xpath, key);
		sr_val_set_str_data(&input[icnt++], SR_STRING_T, val);
	}

	if (sr_connect(SR_CONN_DEFAULT, &conn)) {
		fprintf(stderr, "Error: Connection to datastore failed\n");
		goto err;
	}

	if (sr_session_start(conn, SR_DS_OPERATIONAL, &sess)) {
		fprintf(stderr, "Error: Unable to open transaction to running-config\n");
		goto err_disconnect;
	}

	if ((err = sr_rpc_send(sess, xpath, input, icnt, 0, &output, &ocnt))) {
		fprintf(stderr, "Failed sending RPC %s: %s", xpath, sr_strerror(err));
		goto err_disconnect;
	}

	for (size_t i = 0; i < ocnt; i++) {
		sr_print_val(&output[i]);
		puts("");
	}

	sr_free_values(input, icnt);
	sr_free_values(output, ocnt);
	sr_disconnect(conn);
	return 0;

err_disconnect:
	sr_disconnect(conn);
err:
	sr_free_values(input, icnt);
	return -1;
}

int kplugin_klinfix_fini(kcontext_t *ctx)
{
	return 0;
}

int kplugin_klinfix_init(kcontext_t *ctx)
{
	kplugin_t *plugin = kcontext_plugin(ctx);

	kplugin_add_syms(plugin, ksym_new("klix_copy", klix_copy));
	kplugin_add_syms(plugin, ksym_new("klix_commit", klix_commit));
	kplugin_add_syms(plugin, ksym_new("klix_files", klix_files));
	kplugin_add_syms(plugin, ksym_new("klix_rpc", klix_rpc));

	return 0;
}
