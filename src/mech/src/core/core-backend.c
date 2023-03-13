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

#include <cligen/cligen.h>
#include <clixon/clixon.h>
#include <clixon/clixon_backend.h>

#include <sys/syslog.h>

int core_commit_done(clicon_handle h, transaction_data td)
{
	if (transaction_src(td) && system("initctl reload"))
		clicon_log(LOG_CRIT, "Failed to reload services");

	return 0;
}

static clixon_plugin_api core_api = {
	.ca_name = "core",
	.ca_init = clixon_plugin_init,

	.ca_trans_commit_done = core_commit_done,
};

clixon_plugin_api *clixon_plugin_init(clicon_handle h)
{
	return &core_api;
}
