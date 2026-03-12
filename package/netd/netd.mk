################################################################################
#
# netd
#
################################################################################

NETD_VERSION = 1.1.0
NETD_SITE_METHOD = local
NETD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/netd
NETD_LICENSE = BSD-3-Clause
NETD_LICENSE_FILES = LICENSE
NETD_REDISTRIBUTE = NO
NETD_DEPENDENCIES = libite libconfuse jansson libev
NETD_AUTORECONF = YES

NETD_CONF_ENV = CFLAGS="$(INFIX_CFLAGS)"

NETD_CONF_OPTS = --prefix= --disable-silent-rules

# Backend selection: FRR frr.conf, FRR gRPC, or Linux kernel
ifeq ($(BR2_PACKAGE_NETD_FRR_CONF),y)
NETD_DEPENDENCIES += frr
NETD_CONF_OPTS += --with-frr-conf
else ifeq ($(BR2_PACKAGE_NETD_FRR_VTYSH),y)
NETD_DEPENDENCIES += frr
NETD_CONF_OPTS += --with-frr-vtysh
else ifeq ($(BR2_PACKAGE_NETD_FRR_GRPC),y)
NETD_DEPENDENCIES += frr grpc host-grpc protobuf
NETD_CONF_ENV += \
	PROTOC="$(HOST_DIR)/bin/protoc" \
	GRPC_CPP_PLUGIN="$(HOST_DIR)/bin/grpc_cpp_plugin"
NETD_CONF_OPTS += --with-frr
else
NETD_CONF_OPTS += --without-frr
endif

define NETD_INSTALL_EXTRA
	cp $(NETD_PKGDIR)/tmpfiles.conf $(TARGET_DIR)/etc/tmpfiles.d/netd.conf
endef

NETD_TARGET_FINALIZE_HOOKS += NETD_INSTALL_EXTRA

$(eval $(autotools-package))
