#ifndef STATD_SHARED_H_
#define STATD_SHARED_H_

#include <jansson.h>

#define XPATH_MAX PATH_MAX
#define XPATH_IFACE_BASE "/ietf-interfaces:interfaces"

json_t *json_get_output(const char *cmd);

#endif
