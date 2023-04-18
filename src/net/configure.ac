AC_PREREQ(2.61)
AC_INIT([net], [1.0], [https://github.com/kernelkit/infix/issues])
AC_CONFIG_AUX_DIR(aux)
AM_INIT_AUTOMAKE([1.11 foreign])
AM_SILENT_RULES([yes])

AC_CONFIG_SRCDIR([src/main.c])
AC_CONFIG_HEADERS([config.h])
AC_CONFIG_FILES([Makefile src/Makefile test/Makefile])
AC_CONFIG_MACRO_DIR([m4])

AC_PROG_CC
AC_PROG_INSTALL

# Check for pkg-config first, warn if it's not installed
PKG_PROG_PKG_CONFIG
PKG_CHECK_MODULES([libite],  [libite >= 2.5.0])

AC_OUTPUT

SYSCONFDIR=`eval echo $sysconfdir`
RUNSTATEDIR=`eval echo $runstatedir`
RUNSTATEDIR=`eval echo $RUNSTATEDIR`
cat <<EOF

------------------ Summary ------------------
 $PACKAGE_NAME version $PACKAGE_VERSION
  Prefix.........: $prefix
  Sysconfdir.....: $SYSCONFDIR
  Runstatedir....: $RUNSTATEDIR
  C Compiler.....: $CC $CFLAGS $CPPFLAGS $LDFLAGS $LIBS

------------- Compiler version --------------
$($CC --version || true)
---------------------------------------------

Check the above options and compile with:
 ${MAKE-make}

EOF
