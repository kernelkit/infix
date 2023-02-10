#include <stdio.h>

#include <cligen/cligen.h>
#include <clixon/clixon.h>
#include <clixon/clixon_cli.h>
#include <clixon/cli_generate.h>

int cli_sysinfo(clicon_handle h, cvec *cvv, cvec *argv)
{
	printf("System Information: Infix\n");
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
