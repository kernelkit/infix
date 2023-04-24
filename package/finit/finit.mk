################################################################################
#
# finit
#
################################################################################

FINIT_VERSION = 4.3
FINIT_SITE = https://github.com/troglobit/finit/releases/download/$(FINIT_VERSION)
FINIT_LICENSE = MIT
FINIT_LICENSE_FILES = LICENSE
FINIT_INSTALL_STAGING = YES
FINIT_DEPENDENCIES = host-pkgconf libite libuev
FINIT_INSTALL_STAGING = YES
FINIT_D = $(TARGET_DIR)/etc/finit.d

# Create configure script using autoreconf when building from git
FINIT_VERSION = 700c32fd73ae362af05fb094cb4b069f5147aabf
FINIT_SITE = $(call github,troglobit,finit,$(FINIT_VERSION))
FINIT_AUTORECONF = YES
FINIT_DEPENDENCIES += host-automake host-autoconf host-libtool

# Buildroot defaults to /usr for both prefix and exec-prefix, this we
# must override because we want to install into /sbin and /bin for the
# finit and initctl programs, respectively.  The expected plugin path is
# /lib/finit/ and scripts in /libexec, both are set by --exec-prefix.
# The localstatedir is set to the correct system path by Buildroot, so
# no override necessary there.
FINIT_CONF_OPTS =					\
	--prefix=/usr					\
	--exec-prefix=					\
	--disable-doc					\
	--disable-contrib				\
	--disable-silent-rules				\
	--with-group=$(BR2_PACKAGE_FINIT_INITCTL_GROUP)

ifeq ($(BR2_PACKAGE_FINIT_ADVANCED),y)
ifneq ($(BR2_PACKAGE_FINIT_CUSTOM_FSTAB),)
FINIT_CONF_OPTS += --with-fstab=$(BR2_PACKAGE_FINIT_CUSTOM_FSTAB)
else
FINIT_CONF_OPTS += --without-fstab
endif
endif

ifeq ($(BR2_PACKAGE_FINIT_KEVENTD),y)
FINIT_CONF_OPTS += --with-keventd
else
FINIT_CONF_OPTS += --without-keventd
endif

ifeq ($(BR2_PACKAGE_FINIT_SULOGIN),y)
FINIT_CONF_OPTS += --with-sulogin
else
FINIT_CONF_OPTS += --without-sulogin
endif

ifeq ($(BR2_PACKAGE_FINIT_WATCHDOG),y)
FINIT_CONF_OPTS += --with-watchdog=$(BR2_PACKAGE_FINIT_WATCHDOG_DEV)
else
FINIT_CONF_OPTS += --without-watchdog
endif

ifeq ($(BR2_PACKAGE_FINIT_PLUGIN_HOTPLUG),y)
FINIT_CONF_OPTS += --enable-hotplug-plugin
else
FINIT_CONF_OPTS += --disable-hotplug-plugin
endif

ifeq ($(BR2_PACKAGE_FINIT_PLUGIN_HOOK_SCRIPTS),y)
FINIT_CONF_OPTS += --enable-hook-scripts-plugin
else
FINIT_CONF_OPTS += --disable-hook-scripts-plugin
endif

ifeq ($(BR2_PACKAGE_FINIT_PLUGIN_MODULES_LOAD),y)
FINIT_CONF_OPTS += --enable-modules-load-plugin
else
FINIT_CONF_OPTS += --disable-modules-load-plugin
endif

ifeq ($(BR2_PACKAGE_FINIT_PLUGIN_MODPROBE),y)
FINIT_CONF_OPTS += --enable-modprobe-plugin
else
FINIT_CONF_OPTS += --disable-modprobe-plugin
endif

ifeq ($(BR2_PACKAGE_FINIT_PLUGIN_RTC),y)
FINIT_CONF_OPTS += --enable-rtc-plugin
else
FINIT_CONF_OPTS += --disable-rtc-plugin
endif

ifeq ($(BR2_PACKAGE_FINIT_RTC_DATE),y)
FINIT_CONF_OPTS += --with-rtc-date="$(BR2_PACKAGE_FINIT_RTC_DATE)"
else
FINIT_CONF_OPTS += --without-rtc-date
endif

ifeq ($(BR2_PACKAGE_FINIT_PLUGIN_TTY),y)
FINIT_CONF_OPTS += --enable-tty-plugin
else
FINIT_CONF_OPTS += --disable-tty-plugin
endif

ifeq ($(BR2_PACKAGE_FINIT_PLUGIN_URANDOM),y)
FINIT_CONF_OPTS += --enable-urandom-plugin
else
FINIT_CONF_OPTS += --disable-urandom-plugin
endif

ifneq ($(SKELETON_INIT_COMMON_HOSTNAME),)
FINIT_CONF_OPTS += --with-hostname="$(SKELETON_INIT_COMMON_HOSTNAME)"
endif

# Disable/Enable features depending on other packages
ifeq ($(BR2_PACKAGE_ALSA_UTILS),y)
FINIT_CONF_OPTS += --enable-alsa-utils-plugin
else
FINIT_CONF_OPTS += --disable-alsa-utils-plugin
endif

ifeq ($(BR2_PACKAGE_DBUS),y)
FINIT_CONF_OPTS += --enable-dbus-plugin
else
FINIT_CONF_OPTS += --disable-dbus-plugin
endif

ifeq ($(BR2_PACKAGE_XORG7),y)
FINIT_CONF_OPTS += --enable-x11-common-plugin
else
FINIT_CONF_OPTS += --disable-x11-common-plugin
endif

$(eval $(autotools-package))
