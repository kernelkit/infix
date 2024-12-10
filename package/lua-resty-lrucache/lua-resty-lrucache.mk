################################################################################
#
# lua-resty-lrucache
#
################################################################################
LUA_RESTY_LRUCACHE_VERSION = 0.15
LUA_RESTY_LRUCACHE_MAKE_OPTS += LUA_LIB_DIR=/usr/share/luajit-2.1 PREFIX=/usr
LUA_RESTY_LRUCACHE_LICENSE = BSD-3-Clause
LUA_RESTY_LRUCACHE_SITE = $(call github,openresty,lua-resty-lrucache,v$(LUA_RESTY_LRUCACHE_VERSION))

define LUA_RESTY_LRUCACHE_INSTALL_TARGET_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
	$(LUA_RESTY_LRUCACHE_MAKE_OPTS) DESTDIR="$(TARGET_DIR)" install
endef

$(eval $(generic-package))
