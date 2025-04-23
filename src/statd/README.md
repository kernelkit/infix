# Introduction
Statd is designed to be integrated into Infix and supply the operational
database with data. To do this it uses companion binaries written in
python, these are located in the `python` directory.


## Run outside Infix
### Prerequisites
There are some requirements set on your computer to run statd locally.

 - An Ubuntu based system
 - lldpd
 - python3
 - [libite](https://github.com/troglobit/libite)
 - [libsrx](https://github.com/kernelkit/infix/tree/main/src/libsrx)
 - python-poetry
 - [sysrepo](https://github.com/sysrepo/sysrepo) (At least the same version as Infix)
 - [libyang](https://github.com/CESNET/libyang) (At least the same version as Infix)


### Install YANG modules in local sysrepo
This requires that you first build Infix, since netopeer2 and sysrepo are responsible
for installing their own YANG files.

```bash
user@host ~/infix$ export TARGET_DIR="output/target/"
user@host ~/infix$ export NETOPEER2_SEARCHPATH=$TARGET_DIR/usr/share/yang/modules/netopeer2/
user@host ~/infix$ export SYSREPO_SEARCHPATH=$TARGET_DIR/usr/share/yang/modules/sysrepo/
user@host ~/infix$ export LIBNETCONF2_SEARCHPATH=$TARGET_DIR/usr/share/yang/modules/libnetconf2/
user@host ~/infix$ export CONFD_SEARCHPATH=$TARGET_DIR/usr/share/yang/modules/confd/
user@host ~/infix$ export TEST_MODE_SEARCHPATH=$TARGET_DIR/usr/share/yang/modules/test-mode/
user@host ~/infix$ export ROUSETTE_SEARCHPATH=$TARGET_DIR/usr/share/yang/modules/rousette/
user@host ~/infix$ export SEARCH_PATH="$NETOPEER2_SEARCHPATH $SYSREPO_SEARCHPATH $LIBNETCONF2_SEARCHPATH $CONFD_SEARCHPATH $TEST_MODE_SEARCHPATH $ROUSETTE_SEARCHPATH"

user@host ~/infix$ ./utils/srload src/confd/yang/sysrepo.inc
user@host ~/infix$ ./utils/srload src/confd/yang/libnetconf2.inc
user@host ~/infix$ ./utils/srload src/confd/yang/netopeer2.inc
user@host ~/infix$ ./utils/srload src/confd/yang/confd.inc
user@host ~/infix$ ./utils/srload src/confd/yang/rousette.inc
user@host ~/infix$ ./utils/srload src/confd/yang/test-mode.inc
```

### Build and install python companion binaries
```bash
user@host ~/infix/src/statd/python$ ./local_install.sh
```
This will install the binaries in ~/.local/bin

### Build and install statd

```bash
user@host ~/infix/src/statd$ ./configure --with-yanger-dir=$HOME/.local/bin
user@host ~/infix/src/statd$ make
user@host ~/infix/src/statd$ sudo make install
```

### Running statd
Since the `yanger` binary, for example, reads the shadow database, you
can expect different results if running `statd` as root or not.
```bash
user@host ~/infix/src/statd$ statd
```
