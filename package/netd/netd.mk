################################################################################
#
# netd
#
################################################################################

NETD_VERSION = 1.0
NETD_SITE_METHOD = local
NETD_SITE = $(BR2_EXTERNAL_INFIX_PATH)/src/netd
NETD_LICENSE = BSD-3-Clause
NETD_LICENSE_FILES = LICENSE
NETD_REDISTRIBUTE = NO
NETD_DEPENDENCIES = libite libconfuse jansson
NETD_AUTORECONF = YES

NETD_CONF_ENV = CFLAGS="$(INFIX_CFLAGS)"

NETD_CONF_OPTS = --prefix= --disable-silent-rules

# FRR integration (gRPC backend) or standalone Linux backend
ifeq ($(BR2_PACKAGE_NETD_FRR),y)
NETD_DEPENDENCIES += frr grpc host-grpc protobuf
NETD_CONF_ENV += \
	PROTOC="$(HOST_DIR)/bin/protoc" \
	GRPC_CPP_PLUGIN="$(HOST_DIR)/bin/grpc_cpp_plugin"
else
NETD_CONF_OPTS += --without-frr
endif

NETD_TARGET_FINALIZE_HOOKS += NETD_INSTALL_EXTRA

$(eval $(autotools-package))
