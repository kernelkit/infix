/* SPDX-License-Identifier: BSD-3-Clause */

#include "rip.h"

void rip_config_init(struct rip_config *cfg)
{
	memset(cfg, 0, sizeof(*cfg));

	/* Set defaults */
	cfg->default_metric = 1;
	cfg->distance = 120;
	cfg->timers.update = 30;
	cfg->timers.invalid = 180;
	cfg->timers.flush = 240;

	TAILQ_INIT(&cfg->networks);
	TAILQ_INIT(&cfg->neighbors);
	TAILQ_INIT(&cfg->redistributes);
	TAILQ_INIT(&cfg->system_cmds);
}

void rip_config_free(struct rip_config *cfg)
{
	struct rip_network *net, *net_tmp;
	struct rip_neighbor *nbr, *nbr_tmp;
	struct rip_redistribute *redist, *redist_tmp;

	if (!cfg)
		return;

	TAILQ_FOREACH_SAFE(net, &cfg->networks, entries, net_tmp) {
		TAILQ_REMOVE(&cfg->networks, net, entries);
		free(net);
	}

	TAILQ_FOREACH_SAFE(nbr, &cfg->neighbors, entries, nbr_tmp) {
		TAILQ_REMOVE(&cfg->neighbors, nbr, entries);
		free(nbr);
	}

	TAILQ_FOREACH_SAFE(redist, &cfg->redistributes, entries, redist_tmp) {
		TAILQ_REMOVE(&cfg->redistributes, redist, entries);
		free(redist);
	}

	struct rip_system_cmd *cmd, *cmd_tmp;
	TAILQ_FOREACH_SAFE(cmd, &cfg->system_cmds, entries, cmd_tmp) {
		TAILQ_REMOVE(&cfg->system_cmds, cmd, entries);
		free(cmd);
	}
}
