/* SPDX-License-Identifier: BSD-3-Clause */

#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <string.h>
#include <unistd.h>
#include <termios.h>
#include <getopt.h>
#include <fcntl.h>
#include <syslog.h>
#include <sys/stat.h>
#include <sys/file.h>
#include <time.h>
#include <errno.h>
#include <jansson.h>

static int verbose = 0;

#define MODEMS_LOCK     "/var/lock/modems.lock"
#define MODEMS_INFO     "/run/modems.json"
#define TIMEOUT         3

#define DBG(fmt, a...)      print(LOG_DEBUG, fmt, ##a)
#define SAY(fmt, a...)      print(LOG_INFO, fmt, ##a)
#define ERR(fmt, a...)      print(LOG_ERR, fmt, ##a)
#define FATAL(fmt, a...)    print(LOG_ALERT, fmt, ##a)

static void print (int level, const char *fmt, ...)
{
    char msg[1024];
    va_list ap;

    if (!fmt) return;
    va_start(ap, fmt);
    vsnprintf(msg, sizeof(msg) - 1, fmt, ap);
    va_end(ap);

    switch (level) {
    case LOG_ERR:
        fprintf(stderr, "ERROR: %s\n", msg);
        break;
    case LOG_ALERT:
        fprintf(stderr, "ERROR: %s\n", msg);
        unlink(MODEMS_LOCK);
        exit(1);
        break;
    case LOG_DEBUG:
        if (verbose) {
            printf("DEBUG: %s\n", msg);
        }
        break;
    default:
        printf("%s\n", msg);
        break;
    }
}

static void printser (int dir, char *ser)
{
    char quoted[1024];
    char *p;
    int len = 0;

    for (p = ser; *p; p++) {
        if (*p == '\r') {
            len += snprintf(quoted + len, sizeof(quoted) - len, "<CR>");
        } else if (*p == '\n') {
            len += snprintf(quoted + len, sizeof(quoted) - len, "<NL>");
        } else {
            len += snprintf(quoted + len, sizeof(quoted) - len, "%c", *p);
        }
    }
    if (dir == 0) {
        DBG("Received: '%s'", quoted);
    } else {
        DBG("Sending '%s'", quoted);
    }
}


struct ttymode {
    int     set;
    int     speed;
    int     parity;
    int     bits;
    int     stopbits;
};

static int parse_mode (const char *str, struct ttymode *mode)
{
    char buf[256];
    char *t1, *t2;

    /* parse string */
    if (strlen(str) >= sizeof(buf)) {
        return -1;
    }
    strncpy(buf, str, sizeof(buf));
    t1 = strtok(buf, " ");
    if (!t1) {
        ERR("Invalid format");
        return -1;
    }
    t2 = strtok(NULL, " ");
    if (!t2 || strlen(t2) != 3) {
        ERR("Invalid format");
        return -1;
    }

    /* parse speed */
    switch (atoi(t1)) {
    case 9600:
        mode->speed = B9600;
        break;
    case 19200:
        mode->speed = B19200;
        break;
    case 38400:
        mode->speed = B38400;
        break;
    case 57600:
        mode->speed = B57600;
        break;
    case 115200:
        mode->speed = B115200;
        break;
    default:
        ERR("Invalid baudrate");
        return -1;
    }

    /* parse bits */
    switch (t2[0]) {
    case '5':
        mode->bits = CS5;
        break;
    case '6':
        mode->bits = CS6;
        break;
    case '7':
        mode->bits = CS7;
        break;
    case '8':
        mode->bits = CS8;
        break;
    default:
        ERR("Invalid bits");
        return -1;
    }

    /* parse parity */
    switch (t2[1]) {
    case 'N':
        mode->parity = 0;
        break;
    case 'E':
        mode->parity = PARENB;
        break;
    case 'O':
        mode->parity = PARENB | PARODD;
        break;
    default:
        ERR("Invalid parity");
        return -1;
    }

    /* parse stopbits */
    switch (t2[2]) {
    case '1':
        mode->stopbits = 0;
        break;
    case '2':
        mode->stopbits = CSTOPB;
        break;
    default:
        ERR("Invalid stop bits %c");
        return -1;
    }

    return 0;
}


static int setup (const char *tty, int timeout, struct ttymode *mode)
{
    struct termios stbuf;
    struct stat st;
    char dev[32];
    int i, fd;

    if (strncmp(tty, "/dev", 4) == 0) {
        strncpy(dev, tty, sizeof(dev));
    } else {
        snprintf(dev, sizeof(dev), "/dev/%s", tty);
    }

    /* wait for device */
    for (i = 0; i < timeout; i++) {
        if (lstat(dev, &st) == 0) break;
        sleep(1);
    }
    if (i >= timeout) {
        ERR("Timeout waiting for %s", dev);
        return  -1;
    }

    DBG("Opening %s", tty);
    fd = open(dev, O_RDWR | O_EXCL | O_NONBLOCK | O_NOCTTY);
    if (fd < 0) {
        ERR("Unable to open '%s'", dev);
        return -1;
    }

    if (!mode->set) return fd;

    memset(&stbuf, 0, sizeof (struct termios));
    if (tcgetattr(fd, &stbuf) != 0) {
        ERR("Unable to get serial attributes");
        goto error;
    }
    /* default settings */
    stbuf.c_cflag &= ~(CBAUD | CSIZE | CSTOPB | PARENB | PARODD | CRTSCTS);
    stbuf.c_iflag &= ~(IGNCR | ICRNL | IUCLC | INPCK | IXON | IXOFF | IXANY );
    stbuf.c_oflag &= ~(OPOST | OLCUC | OCRNL | ONLCR | ONLRET);
    stbuf.c_lflag &= ~(ICANON | ECHO | ECHOE | ECHONL);
    stbuf.c_cc[VMIN] = 1;
    stbuf.c_cc[VTIME] = 0;
    stbuf.c_cc[VEOF] = 1;

    /* ignore parity */
    stbuf.c_iflag |= IGNPAR;

    /* configure serial attributes */
    stbuf.c_cflag |= (mode->bits | mode->parity | mode->stopbits | CLOCAL | CREAD);

    /* disable XON/XOFF flow control */
    stbuf.c_iflag &= ~(IXON | IXOFF | IXANY);

    /* disable RTS/CTS flow control */
    stbuf.c_cflag &= ~(CRTSCTS);

    if (cfsetispeed(&stbuf, mode->speed) != 0 ||
        cfsetospeed(&stbuf, mode->speed) != 0) {
        ERR("Unable to set port speed");
        goto error;
    }

    if (tcsetattr(fd, TCSANOW, &stbuf) != 0) {
        ERR("Unable to set serial attributes");
        goto error;
    }

    return fd;
error:
    close(fd);
    return -1;
}

static int serial_write (int fd, int timeout, char *buf, int len)
{
    struct timeval tv;
    fd_set fds;

    FD_ZERO(&fds);
    FD_SET(fd, &fds);
    tv.tv_sec = timeout;
    tv.tv_usec = 0;

    if (select(fd + 1, NULL, &fds, NULL, &tv) <= 0) {
        DBG("Cannot write to serial device");
        return -1;
    }
    alarm(timeout);
    tcflush(fd, TCIOFLUSH);

    printser(1, buf);
    if (write(fd, buf, len) != len) {
        return -1;
    }
    tcdrain(fd);
    alarm(0);

    return 0;
}

static int serial_read (int fd, int timeout, char *buf, int size)
{
    struct timeval tv;
    fd_set fds;
    int rc, len = 0;

    FD_ZERO(&fds);
    FD_SET(fd, &fds);
    tv.tv_sec = timeout;
    tv.tv_usec = 0;
    memset(buf, 0, size);

    rc = select(fd + 1, &fds, NULL, NULL, &tv);
    if (rc <= 0) {
        DBG("Cannot read from serial device (%d)", rc);
        return -1;
    }
    do {
        errno = 0;
        alarm(timeout);
        rc = read(fd, buf+len, size-len-1);
        alarm(0);

        if (rc > 0) {
            len += rc;
        } else {
            if (rc < 0 && errno == -EAGAIN) {
                continue;
            } else {
                break;
            }
        }
    } while (1);

    if (len > 0) buf[len] = '\0';

    printser(0, buf);

    return len;
}

static int execute (int fd, int timeout, const char *cmd, const char *expect)
{
    char buf[1024];
    time_t start;
    long elapsed;
    int len, ret = -1;

    if (cmd) {
        start = time(NULL);
        len = snprintf(buf, sizeof(buf), "%s\r\n", cmd);
        if (serial_write(fd, timeout, buf, len) < 0) {
            DBG("Cannot write to serial device");
            return -1;
        }
        elapsed = time(NULL) - start;
        timeout -= elapsed;
    }

    DBG("Waiting for response");

    while (timeout > 0) {
        start = time(NULL);
        len = serial_read(fd, timeout, buf, sizeof(buf));
        if (len > 0) {
            buf[len] = '\0';
            SAY("%s", buf);

            if (strstr(buf, expect)) {
                ret = 0;
                break;
            }
        }
        elapsed = time(NULL) - start;
        timeout -= elapsed;
    }
    return ret;
}

static int probe (int fd)
{
    char buf[256];
    int i, len;

    DBG("Probing serial port");

    /* try 3 times */
    for (i = 3; i > 0; i--) {
        len = snprintf(buf, sizeof(buf), "AT\r\n");
        if (serial_write(fd, 1, buf, len) < 0) {
            DBG("Cannot write to tty");
            continue;
        }
        len = serial_read(fd, 1, buf, sizeof(buf));
        if (len <= 0) {
            DBG("Cannot read from tty");
            continue;
        }
        if (strstr(buf, "OK")) {
            return 0;
        }
    }

    DBG("Probing failed");
    return -1;
}

static int modemidx (const char *devpath)
{
    json_t *json, *modules, *module, *mod, *paths, *obj;
    const char *p;
    int i, j, index = -1;

    json = json_load_file("/run/modules.json", 0, NULL);
    if (!json) {
        return -1;
    }
    modules = json_object_get(json, "modules");
    if (!modules || !json_is_array(modules)) {
        goto out;
    }
    for (i = 0; i < json_array_size(modules); i++) {
        module = json_array_get(modules, i);
        if (!module) continue;

        obj = json_object_get(module, "type");
        if (!obj || !json_is_string(obj)) continue;
        p = json_string_value(obj);
        if (strcmp(p, "modem") != 0) continue;

        obj = json_object_get(module, "index");
        if (!obj || !json_is_integer(obj)) continue;
        index = json_integer_value(obj);

        paths = json_object_get(module, "paths");
        if (!paths || !json_is_array(paths)) continue;

        for (j = 0; j < json_array_size(paths); j++) {
            obj = json_array_get(paths, j);
            if (!obj) continue;
            p = json_string_value(obj);

            if (strstr(devpath, p)) {
                /* found */
                goto out;
            }
        }
    }
    index = -1;
out:
    json_decref(json);
    return index;
}

static char *modemdev (int index, int port)
{
    json_t *json, *modems, *modem, *ports, *obj;
    char *dev = NULL;
    int i, p, fd;

    fd = open(MODEMS_LOCK,
               O_WRONLY | O_CREAT | O_EXCL,
               S_IRUSR | S_IWUSR | S_IRGRP | S_IROTH);
    if (fd < 0) {
        ERR("Cannot open lock");
        return NULL;
    }
    if (flock(fd, LOCK_EX) < 0) {
        ERR("Cannot to lock");
        close(fd);
        return NULL;
    }

    json = json_load_file(MODEMS_INFO, 0, NULL);
    if (!json) {
        goto out;
    }
    modems = json_object_get(json, "modems");
    if (!modems || !json_is_array(modems)) {
        goto out;
    }
    for (i = 0; i < json_array_size(modems); i++) {
        modem = json_array_get(modems, i);
        if (!modem)
            continue;

        obj = json_object_get(modem, "devpath");
        if (!obj || !json_is_string(obj))
            continue;

        if (modemidx(json_string_value(obj)) != index)
            continue;

        ports = json_object_get(modem, "atports");
        if (!ports || !json_is_array(ports) || json_array_size(ports) == 0)
            goto out;

        if (json_array_size(ports) >= port)
            port = 0;

        obj = json_array_get(ports, port);
        if (!obj || !json_is_string(obj))
            goto out;

        /* found */
        dev = strdup(json_string_value(obj));
        goto out;
    }
    dev = NULL;

out:
    if (json) json_decref(json);

    unlink(MODEMS_LOCK);
    flock(fd, LOCK_UN);
    close(fd);

    return dev;
}

static struct option options[] = {
    { "index",      required_argument, 0,    'i' },
    { "device",     required_argument, 0,    'd' },
    { "mode",       required_argument, 0,    'm' },
    { "command",    required_argument, 0,    'c' },
    { "expect",     required_argument, 0,    'e' },
    { "timeout",    required_argument, 0,    't' },
    { "primary",    no_argument,       0,    'P' },
    { "secondary",  no_argument,       0,    'S' },
    { "verbose",    no_argument,       0,    'v' },
    { "help",       no_argument,       0,    'h' },
    { 0, 0, 0, 0 }
};

static void usage (void)
{
    fprintf(stderr, "Usage: modem-command [OPTIONS] [command]\n\n");
    fprintf(stderr, "   --index <idx>       Index of modem\n");
    fprintf(stderr, "   --device <tty>      TTY device of modem\n");
    fprintf(stderr, "   --mode <mode>       Serial mode for TTY (e.g. '115200 8N1')\n");
    fprintf(stderr, "   --expect <str>      String to expect\n");
    fprintf(stderr, "   --timeout <to>      Timeout for operation\n");
    fprintf(stderr, "   --primary           Use primary atport\n");
    fprintf(stderr, "   --secondary         Use secondary atport\n");
    fprintf(stderr, "   --verbose           Be verbose\n");
    fprintf(stderr, "   --help              print help\n\n");
    exit(1);
}

int main (int argc, char *argv[])
{
    const char *cmd = NULL, *expect = "OK";
    char *dev = NULL;
    struct ttymode mode;
    int timeout = TIMEOUT;
    int index = 0, port = 0;
    int opt, c, fd;
    int ret = 1;

    memset(&mode, 0, sizeof(struct ttymode));

    while (1) {
        c = getopt_long(argc, argv, "i:d:m:e:t:PSvh", options, &opt);
        if (c < 0) break;

        switch (c) {
        case 'i':
            index = atoi(optarg);
            break;
        case 'd':
            dev = strdup(optarg);
            break;
        case 'm':
            if (parse_mode(optarg, &mode) < 0) {
                FATAL("Invalid serial mode specified");
            }
            mode.set = 1;
            break;
        case 'c':
            cmd = optarg;
            break;
        case 'e':
            expect = optarg;
            break;
        case 't':
            timeout = atoi(optarg);
            break;
        case 'P':
            port = 0;
            break;
        case 'S':
            port = 1;
            break;
        case 'v':
            verbose = 1;
            break;
        default:
            usage();
            break;
        }
    }

    if (optind < argc) {
        cmd = argv[optind];
        if (strlen(cmd) == 0)
            FATAL("Invalid command");
    }
    dev = modemdev(index, port);
    if (!dev || strlen(dev) == 0) {
        FATAL("No device found");
    }
    fd = setup(dev, timeout, &mode);
    if (fd < 0) {
        FATAL("Unable to setup device");
    }

    if (probe(fd) < 0) {
        ERR("Unable to probe device");
        goto abort;
    }
    if (execute(fd, timeout, cmd, expect) < 0) {
        ERR("Command failed");
        goto abort;
    }
    ret = 0;

abort:
    close(fd);
    if (dev) free(dev);
    return ret;
}
