FROM alpine:3.18.0

RUN apk add --no-cache \
    busybox-extras \
    e2fsprogs \
    fakeroot \
    gcc \
    graphviz \
    iproute2 \
    iputils \
    libc-dev \
    libyang-dev \
    linux-headers \
    python3-dev \
    qemu-img \
    qemu-system-x86_64 \
    ruby-mustache \
    socat \
    squashfs-tools \
    tcpdump

# Alpine's QEMU package does not bundle this for some reason, copied
# from Ubuntu
COPY qemu-ifup /etc

# Needed to let qeneth find mustache(1)
ENV PATH="${PATH}:/usr/lib/ruby/gems/3.2.0/bin"

# Install all python packages used by the tests
COPY init-venv.sh /root
COPY pip-requirements.txt /root
RUN ~/init-venv.sh ~/pip-requirements.txt
