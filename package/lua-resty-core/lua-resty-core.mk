################################################################################
#
# lua-resty-core
#
################################################################################
LUA_RESTY_CORE_VERSION = 0.1.30
LUA_RESTY_CORE_MAKE_OPTS += LUA_LIB_DIR=/usr/share/luajit-2.1 PREFIX=/usr
LUA_RESTY_CORE_LICENSE = BSD-3-Clause
LUA_RESTY_CORE_SITE = $(call github,openresty,lua-resty-core,v$(LUA_RESTY_CORE_VERSION))

define LUA_RESTY_CORE_INSTALL_TARGET_CMDS
	$(TARGET_MAKE_ENV) $(TARGET_CONFIGURE_OPTS) $(MAKE) -C $(@D) \
	$(LUA_RESTY_CORE_MAKE_OPTS) DESTDIR="$(TARGET_DIR)" install
endef


$(eval $(generic-package))
