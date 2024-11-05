################################################################################
#
# finit
#
################################################################################

FINIT_VERSION = 4.8
FINIT_SITE = https://github.com/troglobit/finit/releases/download/$(FINIT_VERSION)
FINIT_LICENSE = MIT
FINIT_LICENSE_FILES = LICENSE
FINIT_INSTALL_STAGING = YES
FINIT_DEPENDENCIES = host-pkgconf libite libuev
FINIT_INSTALL_STAGING = YES
FINIT_D = $(TARGET_DIR)/etc/finit.d

# Create configure script using autoreconf when building from git,
# or when patching any of the GNU build files (*.ac, *.am, etc.)
#FINIT_VERSION = 438d6b4e638418a2a22024a3cead2f47909d72b9
#FINIT_SITE = $(call github,troglobit,finit,$(FINIT_VERSION))
FINIT_AUTORECONF = YES
FINIT_DEPENDENCIES += host-automake host-autoconf host-libtool

# Strip "" from variables
FINIT_HOSTNAME = $(call qstrip,$(BR2_TARGET_GENERIC_HOSTNAME))
FINIT_GROUP    = $(call qstrip,$(BR2_PACKAGE_FINIT_INITCTL_GROUP))
FINIT_FSTAB    = $(call qstrip,$(BR2_PACKAGE_FINIT_CUSTOM_FSTAB))
FINIT_RTC_DATE = $(call qstrip,$(BR2_PACKAGE_FINIT_RTC_DATE))
FINIT_RTC_FILE = $(call qstrip,$(BR2_PACKAGE_FINIT_RTC_FILE))

# Buildroot defaults to /usr for both prefix and exec-prefix, this we
# must override because we want to install into /sbin and /bin for the
# finit and initctl programs, respectively.  The expected plugin path is
# /lib/finit/ and scripts in /libexec, both are set by --exec-prefix.
# The localstatedir is set to the correct system path by Buildroot, so
# no override necessary there.
FINIT_CONF_OPTS =					\
	--prefix=/usr					\
	--disable-doc					\
	--disable-contrib				\
	--disable-rescue				\
	--disable-silent-rules				\
	--with-group="$(FINIT_GROUP)"

ifeq ($(BR2_ROOTFS_MERGED_USR),y)
FINIT_CONF_OPTS += --exec-prefix=/usr
else
FINIT_CONF_OPTS += --exec-prefix=
endif

ifeq ($(BR2_PACKAGE_FINIT_ADVANCED),y)
ifneq ($(FINIT_FSTAB),)
FINIT_CONF_OPTS += --with-fstab="$(FINIT_FSTAB)"
else
FINIT_CONF_OPTS += --without-fstab
endif
endif

ifeq ($(BR2_PACKAGE_FINIT_SILENCE_KERNEL),y)
FINIT_CONF_OPTS += --disable-kernel-logging
else
FINIT_CONF_OPTS += --enable-kernel-logging
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

ifneq ($(FINIT_RTC_DATE),)
FINIT_CONF_OPTS += --with-rtc-date="$(FINIT_RTC_DATE)"
else
FINIT_CONF_OPTS += --without-rtc-date
endif

ifneq ($(FINIT_RTC_FILE),)
FINIT_CONF_OPTS += --with-rtc-file="$(FINIT_RTC_FILE)"
else
FINIT_CONF_OPTS += --without-rtc-file
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


ifneq ($(FINIT_HOSTNAME),)
FINIT_CONF_OPTS += --with-hostname="$(FINIT_HOSTNAME)"
else
FINIT_CONF_OPTS += --without-hostname
endif

# Disable/Enable features depending on other packages
ifeq ($(BR2_PACKAGE_ALSA_UTILS),y)
FINIT_CONF_OPTS += --enable-alsa-utils-plugin
else
FINIT_CONF_OPTS += --disable-alsa-utils-plugin
endif

ifeq ($(BR2_PACKAGE_BASH_COMPLETION),y)
FINIT_CONF_OPTS += --with-bash-completiond-dir
else
FINIT_CONF_OPTS += --without-bash-completiond-dir
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
