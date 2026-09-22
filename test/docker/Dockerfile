FROM alpine:3.18.0

# NOTE: please add packages alphabetically!
RUN apk add --no-cache \
    bash \
    bison \
    busybox-extras \
    chrony \
    coreutils \
    curl \
    dhcp-server-vanilla \
    dnsmasq \
    e2fsprogs \
    e2tools \
    ethtool \
    fakeroot \
    file \
    flex \
    font-dejavu \
    gcc \
    git \
    graphviz \
    iperf3 \
    iproute2 \
    iputils \
    jq \
    libc-dev \
    libnet-dev \
    libnetfilter_conntrack-dev \
    libnl3-dev \
    libpcap-dev \
    libyang-dev \
    linux-headers \
    make \
    ncurses-dev \
    net-snmp-tools \
    nmap \
    openssh-client \
    openssl \
    pandoc-cli \
    pkgconf \
    python3-dev \
    qemu-img \
    qemu-system-x86_64 \
    ruby-mustache \
    skopeo \
    socat \
    squashfs-tools \
    sshpass \
    tcpdump \
    tshark \
    userspace-rcu-dev \
    weasyprint \
    zlib-dev

ARG MTOOL_VERSION="3.0"
RUN wget https://github.com/troglobit/mtools/releases/download/v3.0/mtools-$MTOOL_VERSION.tar.gz -O /tmp/mtools-$MTOOL_VERSION.tar.gz
RUN cd /tmp/ && tar zxvf mtools-$MTOOL_VERSION.tar.gz
RUN cd /tmp/mtools-$MTOOL_VERSION && make && make install

# trafgen and mausezahn, traffic generators for the QoS tests.  Alpine
# has no netsniff-ng package, and mausezahn needs libcli, which is only
# in edge/testing, so both are built from their release tarballs.
ARG LIBCLI_VERSION="1.10.7"
RUN wget https://github.com/dparrish/libcli/archive/refs/tags/V$LIBCLI_VERSION.tar.gz -O /tmp/libcli-$LIBCLI_VERSION.tar.gz
RUN cd /tmp/ && tar zxf libcli-$LIBCLI_VERSION.tar.gz
RUN cd /tmp/libcli-$LIBCLI_VERSION && make && make install PREFIX=/usr

ARG NETSNIFF_NG_VERSION="0.6.9"
RUN wget https://github.com/netsniff-ng/netsniff-ng/archive/refs/tags/v$NETSNIFF_NG_VERSION.tar.gz -O /tmp/netsniff-ng-$NETSNIFF_NG_VERSION.tar.gz
RUN cd /tmp/ && tar zxf netsniff-ng-$NETSNIFF_NG_VERSION.tar.gz
RUN cd /tmp/netsniff-ng-$NETSNIFF_NG_VERSION && ./configure --disable-geoip && \
    make trafgen mausezahn && make trafgen_install mausezahn_install

# Alpine's QEMU package does not bundle this for some reason, copied
# from Ubuntu
COPY docker/qemu-ifup /etc

# Needed to let qeneth find mustache(1)
ENV PATH="${PATH}:/usr/lib/ruby/gems/3.2.0/bin"

# Install all python packages used by the tests
COPY docker/init-venv.sh /root
COPY docker/pip-requirements.txt /root

# Add bootstrap YANG models, the rest will be downloaded from the device
ADD docker/yang /root/yang

RUN ~/init-venv.sh ~/pip-requirements.txt

# Download container images for upgrade testing
COPY case/containers/upgrade/download.sh /tmp/download.sh
RUN /tmp/download.sh /srv && rm /tmp/download.sh

COPY docker/entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/sh"]
