IXMSG = printf "\e[37;44m>>>   $(call qstrip,$(1))\e[0m\n"

oem-dir := $(call qstrip,$(INFIX_OEM_PATH))
INFIX_TOPDIR = $(if $(oem-dir),$(oem-dir),$(BR2_EXTERNAL_INFIX_PATH))

# Unless the user specifies an explicit build id, source it from git.
# The build id also becomes the image version, unless an official
# release is being built.
export INFIX_BUILD_ID ?= $(shell git -C $(INFIX_TOPDIR) describe --dirty --always --tags)
export INFIX_VERSION = $(if $(INFIX_RELEASE),$(INFIX_RELEASE),$(INFIX_BUILD_ID))

INFIX_CFLAGS:=-Wall -Werror -Wextra -Wno-unused-parameter -Wformat=2 -Wformat-overflow=2 -Winit-self -Wstrict-overflow=4 -Wno-format-truncation -Wno-format-nonliteral
