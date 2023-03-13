/*
 * Copyright 2023  The KernelKit Authors <kernelkit@googlegroups.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include <glob.h>
#include <stdio.h>

#include <cligen/cligen.h>
#include <clixon/clixon.h>
#include <clixon/clixon_cli.h>
#include <clixon/cli_generate.h>

#define _PATH_LOG "/var/log/"

/* Not exported in the common Clixon API, needed to run external commands */
void cli_signal_block(clicon_handle h);
void cli_signal_unblock(clicon_handle h);

static int exec(clicon_handle h, char *cmd)
{
	int rc;

	cli_signal_unblock(h);
	rc = system(cmd);
	cli_signal_block(h);

	return rc;
}

int cli_exec_shell(clicon_handle h, cvec *cvv, cvec *argv)
{
	return exec(h, "$SHELL -l");
}

int cli_exec_show(clicon_handle h, cvec *cvv, cvec *argv)
{
	char cmd[256];
	int pos = 0;
	int i;

	for (i = 0; i < cvec_len(cvv); i++)
		pos += snprintf(&cmd[pos], sizeof(cmd) - pos, "%s ",
				cv_string_get(cvec_i(cvv, i)));

	return exec(h, cmd);
}

int cli_exec_log(clicon_handle h, cvec *cvv, cvec *argv)
{
	char cmd[128], arg[32];
	cg_var *cv;

	/* cvv[0] is the whole command, e.g. 'show log btmp' */
	if (!strncmp("follow", cv_string_get(cvec_i(cvv, 0)), 6))
		snprintf(cmd, sizeof(cmd), "tail -F ");
	else
		snprintf(cmd, sizeof(cmd), "cat ");

	cv = cvec_find(cvv, "fn");
	if (cv) {
		snprintf(arg, sizeof(arg), _PATH_LOG "%s", cv_string_get(cv));
		strcat(cmd, arg);
	} else
		strcat(cmd, _PATH_LOG "syslog");

	return exec(h, cmd);
}

int logfiles(cligen_handle h, char *fn_str, cvec *cvv, cg_var *argv,
	     cvec *commands, cvec *helptexts)
{
	glob_t gl;
	size_t i;
	int rc;

	rc = glob(_PATH_LOG "*", 0, NULL, &gl);
	if (rc && rc != GLOB_NOMATCH)
		return 0;

	for (i = 0; i < gl.gl_pathc; i++) {
		char *fn;

		fn = rindex(gl.gl_pathv[i], '/');
		if (fn)
			fn++;
		else
			fn = gl.gl_pathv[i];
		cvec_add_string(commands, NULL, fn);
	}
	globfree(&gl);

	return 0;
}

static clixon_plugin_api cli_api = {
	.ca_name = "infix-cli",
	.ca_init = clixon_plugin_init,
};

clixon_plugin_api *clixon_plugin_init(clicon_handle h)
{
	return &cli_api;
}
