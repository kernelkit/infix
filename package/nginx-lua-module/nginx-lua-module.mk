################################################################################
#
# nginx-lua-module
#
################################################################################

NGINX_LUA_MODULE_VERSION = v0.10.27
NGINX_LUA_MODULE_SITE = $(call github,openresty,lua-nginx-module,$(NGINX_LUA_MODULE_VERSION))
NGINX_LUA_MODULE_LICENSE = BSD-3-Clause
NGINX_LUA_MODULE_DEPENDENCIES = luainterpreter lua-resty-core

$(eval $(generic-package))
