FROM alpine:3.18.0

# NOTE: please add packages alphabetically!
RUN apk add --no-cache \
    busybox-extras \
    curl \
    dhcp-server-vanilla \
    dnsmasq \
    e2fsprogs \
    e2tools \
    ethtool \
    fakeroot \
    file \
    gcc \
    git \
    graphviz \
    iproute2 \
    iputils \
    jq \
    libc-dev \
    libyang-dev \
    linux-headers \
    make \
    nmap \
    openssh-client \
    openssl \
    python3-dev \
    qemu-img \
    qemu-system-x86_64 \
    ruby-mustache \
    skopeo \
    socat \
    squashfs-tools \
    sshpass \
    tcpdump \
    tshark

ARG MTOOL_VERSION="3.0"
RUN wget https://github.com/troglobit/mtools/releases/download/v3.0/mtools-$MTOOL_VERSION.tar.gz -O /tmp/mtools-$MTOOL_VERSION.tar.gz
RUN cd /tmp/ && tar zxvf mtools-$MTOOL_VERSION.tar.gz
RUN cd /tmp/mtools-$MTOOL_VERSION && make && make install

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
COPY case/infix_containers/upgrade/download.sh /tmp/download.sh
RUN /tmp/download.sh /srv && rm /tmp/download.sh

COPY docker/entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/sh"]
