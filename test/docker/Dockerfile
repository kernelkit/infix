FROM alpine:3.18.0

RUN apk add --no-cache \
    busybox-extras \
    e2fsprogs \
    fakeroot \
    gcc \
    graphviz \
    iproute2 \
    iputils \
    jq \
    libc-dev \
    libyang-dev \
    linux-headers \
    openssh-client \
    python3-dev \
    qemu-img \
    qemu-system-x86_64 \
    ruby-mustache \
    socat \
    squashfs-tools \
    sshpass \
    tcpdump \
    tshark \
    openssl \
    curl \
    make

ARG MTOOL_VERSION="3.0"
RUN wget https://github.com/troglobit/mtools/releases/download/v3.0/mtools-$MTOOL_VERSION.tar.gz -O /tmp/mtools-$MTOOL_VERSION.tar.gz
RUN cd /tmp/ && tar zxvf mtools-$MTOOL_VERSION.tar.gz
RUN cd /tmp/mtools-$MTOOL_VERSION && make && make install

# Alpine's QEMU package does not bundle this for some reason, copied
# from Ubuntu
COPY qemu-ifup /etc

# Needed to let qeneth find mustache(1)
ENV PATH="${PATH}:/usr/lib/ruby/gems/3.2.0/bin"

# Install all python packages used by the tests
COPY init-venv.sh /root
COPY pip-requirements.txt /root

# Add bootstrap YANG models, the rest will be downloaded from the device
ADD yang /root/yang

RUN ~/init-venv.sh ~/pip-requirements.txt

COPY entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
CMD ["/bin/sh"]
