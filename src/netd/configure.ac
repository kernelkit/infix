AC_PREREQ(2.61)
AC_INIT([netd], [1.0.0], [https://github.com/kernelkit/infix/issues])
AM_INIT_AUTOMAKE(1.11 foreign subdir-objects)
AM_SILENT_RULES(yes)

AC_CONFIG_FILES([
	Makefile
])

AC_PROG_CC
AC_PROG_CXX
AC_PROG_INSTALL

#
# FRR gRPC northbound API (optional, enabled by default)
#
AC_ARG_WITH(frr,
        AS_HELP_STRING([--without-frr], [Build without FRR integration (use Linux kernel backend)]),
	[with_frr=$withval],
	[with_frr=yes])

AS_IF([test "x$with_frr" = "xyes"], [
        # Check for protoc and grpc_cpp_plugin
        AC_PATH_PROG([PROTOC], [protoc], [no])
        AS_IF([test "x$PROTOC" = "xno"], [
                AC_MSG_ERROR([protoc not found, required for FRR support (use --without-frr to disable)])
        ])

        AC_PATH_PROG([GRPC_CPP_PLUGIN], [grpc_cpp_plugin], [no])
        AS_IF([test "x$GRPC_CPP_PLUGIN" = "xno"], [
                AC_MSG_ERROR([grpc_cpp_plugin not found, required for FRR support (use --without-frr to disable)])
        ])

        # Check for grpc++ and protobuf libraries
        PKG_CHECK_MODULES([grpc], [grpc++ >= 1.16.0])
        PKG_CHECK_MODULES([protobuf], [protobuf >= 3.0.0])

        AC_DEFINE(HAVE_FRR_GRPC, 1, [Built with FRR gRPC northbound API support])

        AC_SUBST(PROTOC)
        AC_SUBST(GRPC_CPP_PLUGIN)
])

AM_CONDITIONAL(HAVE_FRR_GRPC, [test "x$with_frr" = "xyes"])

# Check for pkg-config first
PKG_PROG_PKG_CONFIG

PKG_CHECK_MODULES([libite], [libite >= 2.6.1])
PKG_CHECK_MODULES([libconfuse], [libconfuse >= 3.0])
PKG_CHECK_MODULES([jansson], [jansson >= 2.0])

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
  FRR integration.......: $with_frr
  Backend...............: $(test "x$with_frr" = "xyes" && echo "FRR gRPC" || echo "Linux kernel")
  C Compiler............: $CC $CFLAGS $CPPFLAGS $LDFLAGS $LIBS

------------- Compiler version --------------
$($CC --version || true)
---------------------------------------------

Check the above options and compile with:
 ${MAKE-make}

EOF
