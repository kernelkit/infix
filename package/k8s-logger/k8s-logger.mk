################################################################################
#
# k8s-logger
#
################################################################################

K8S_LOGGER_VERSION = 1.3
K8S_LOGGER_SITE = https://github.com/kernelkit/k8s-logger/releases/download/v$(K8S_LOGGER_VERSION)
K8S_LOGGER_LICENSE = MIT
K8S_LOGGER_LICENSE_FILES = LICENSE
K8S_LOGGER_DEPENDENCIES = sysklogd libite
K8S_LOGGER_CONF_OPTS = --with-syslogp

$(eval $(autotools-package))
