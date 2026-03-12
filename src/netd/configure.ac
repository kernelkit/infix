AC_PREREQ(2.61)
AC_INIT([netd], [1.1.0], [https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(yes)

AC_CONFIG_FILES([
	Makefile
])

AC_PROG_CC
AC_PROG_CXX
AC_PROG_INSTALL

#
# FRR backend selection
#
AC_ARG_WITH(frr,
        AS_HELP_STRING([--with-frr], [Build with FRR gRPC backend]),
	[with_frr=$withval],
	[with_frr=no])

AC_ARG_WITH(frr-conf,
        AS_HELP_STRING([--with-frr-conf], [Build with FRR frr.conf file backend (default when FRR available)]),
	[with_frr_conf=$withval],
	[with_frr_conf=no])

AC_ARG_WITH(frr-vtysh,
        AS_HELP_STRING([--with-frr-vtysh], [Build with FRR vtysh -b backend (incremental config)]),
	[with_frr_vtysh=$withval],
	[with_frr_vtysh=no])

# Mutual exclusion check
AS_IF([test "x$with_frr" = "xyes" -a "x$with_frr_conf" = "xyes"], [
        AC_MSG_ERROR([--with-frr and --with-frr-conf are mutually exclusive])
])
AS_IF([test "x$with_frr" = "xyes" -a "x$with_frr_vtysh" = "xyes"], [
        AC_MSG_ERROR([--with-frr and --with-frr-vtysh are mutually exclusive])
])
AS_IF([test "x$with_frr_conf" = "xyes" -a "x$with_frr_vtysh" = "xyes"], [
        AC_MSG_ERROR([--with-frr-conf and --with-frr-vtysh are mutually exclusive])
])

AS_IF([test "x$with_frr" = "xyes"], [
        # Check for protoc and grpc_cpp_plugin
        AC_PATH_PROG([PROTOC], [protoc], [no])
        AS_IF([test "x$PROTOC" = "xno"], [
                AC_MSG_ERROR([protoc not found, required for FRR gRPC support])
        ])

        AC_PATH_PROG([GRPC_CPP_PLUGIN], [grpc_cpp_plugin], [no])
        AS_IF([test "x$GRPC_CPP_PLUGIN" = "xno"], [
                AC_MSG_ERROR([grpc_cpp_plugin not found, required for FRR gRPC support])
        ])

        # Check for grpc++ and protobuf libraries
        PKG_CHECK_MODULES([grpc], [grpc++ >= 1.16.0])
        PKG_CHECK_MODULES([protobuf], [protobuf >= 3.0.0])

        AC_DEFINE(HAVE_FRR_GRPC, 1, [Built with FRR gRPC northbound API support])

        AC_SUBST(PROTOC)
        AC_SUBST(GRPC_CPP_PLUGIN)
])

AS_IF([test "x$with_frr_conf" = "xyes"], [
        AC_DEFINE(HAVE_FRR_CONF, 1, [Built with FRR frr.conf file backend])
])

AS_IF([test "x$with_frr_vtysh" = "xyes"], [
        AC_DEFINE(HAVE_FRR_VTYSH, 1, [Built with FRR vtysh -b backend])
])

AM_CONDITIONAL(HAVE_FRR_GRPC, [test "x$with_frr" = "xyes"])
AM_CONDITIONAL(HAVE_FRR_CONF, [test "x$with_frr_conf" = "xyes"])
AM_CONDITIONAL(HAVE_FRR_VTYSH, [test "x$with_frr_vtysh" = "xyes"])

# Check for pkg-config first
PKG_PROG_PKG_CONFIG

PKG_CHECK_MODULES([libite], [libite >= 2.6.1])
PKG_CHECK_MODULES([libconfuse], [libconfuse >= 3.0])
PKG_CHECK_MODULES([jansson], [jansson >= 2.0])

AC_CHECK_HEADER([ev.h],
    [saved_LIBS="$LIBS"
     AC_CHECK_LIB([ev], [ev_loop_new],
        [EV_LIBS="-lev"],
        [AC_MSG_ERROR("libev not found")])
     LIBS="$saved_LIBS"],
    [AC_MSG_ERROR("ev.h not found")]
)
AC_SUBST([EV_LIBS])

test "x$prefix" = xNONE && prefix=$ac_default_prefix
test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

DATAROOTDIR=`eval echo $datarootdir`
DATAROOTDIR=`eval echo $DATAROOTDIR`
AC_SUBST(DATAROOTDIR)

SYSCONFDIR=`eval echo $sysconfdir`
SYSCONFDIR=`eval echo $SYSCONFDIR`
AC_SUBST(SYSCONFDIR)

RUNSTATEDIR=`eval echo $runstatedir`
RUNSTATEDIR=`eval echo $RUNSTATEDIR`
AC_SUBST(RUNSTATEDIR)

AC_OUTPUT

cat <<EOF

------------------ Summary ------------------
 $PACKAGE_NAME version $PACKAGE_VERSION
  Prefix................: $prefix
  Exec prefix...........: $eprefix
  Sysconfdir............: `eval echo $sysconfdir`
  Backend...............: $(test "x$with_frr" = "xyes" && echo "FRR gRPC" || (test "x$with_frr_conf" = "xyes" && echo "FRR frr.conf" || (test "x$with_frr_vtysh" = "xyes" && echo "FRR vtysh" || echo "Linux kernel")))
  C Compiler............: $CC $CFLAGS $CPPFLAGS $LDFLAGS $LIBS

------------- Compiler version --------------
$($CC --version || true)
---------------------------------------------

Check the above options and compile with:
 ${MAKE-make}

EOF
