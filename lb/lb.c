#define _GNU_SOURCE
#include <arpa/inet.h>
#include <errno.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <poll.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <time.h>
#include <unistd.h>

#define MAX_WORKERS 8
#define ACCEPT_BATCH 1024

typedef struct {
    int fd;
    char dummy;
    struct iovec iov;
    union {
        struct cmsghdr cm;
        char buf[CMSG_SPACE(sizeof(int))];
    } ctl;
    struct msghdr msg;
    struct cmsghdr *cmsg;
} worker_t;

static void init_worker(worker_t *w, int fd) {
    memset(w, 0, sizeof(*w));
    w->fd = fd;
    w->dummy = 1;
    w->iov.iov_base = &w->dummy;
    w->iov.iov_len = 1;
    w->msg.msg_iov = &w->iov;
    w->msg.msg_iovlen = 1;
    w->msg.msg_control = w->ctl.buf;
    w->msg.msg_controllen = sizeof(w->ctl.buf);
    w->cmsg = CMSG_FIRSTHDR(&w->msg);
    w->cmsg->cmsg_level = SOL_SOCKET;
    w->cmsg->cmsg_type = SCM_RIGHTS;
    w->cmsg->cmsg_len = CMSG_LEN(sizeof(int));
}

static int send_fd(worker_t *w, int fd, int flags) {
    w->msg.msg_controllen = sizeof(w->ctl.buf);
    memcpy(CMSG_DATA(w->cmsg), &fd, sizeof(int));
    for (;;) {
        ssize_t r = sendmsg(w->fd, &w->msg, MSG_NOSIGNAL | flags);
        if (r > 0) return 0;
        if (r < 0 && errno == EINTR) continue;
        return -1;
    }
}

static int connect_worker(const char *path) {
    struct stat st;
    for (int t = 0; t < 600 && stat(path, &st) != 0; t++) {
        struct timespec ts = {0, 100 * 1000 * 1000};
        nanosleep(&ts, NULL);
    }
    for (int t = 0; t < 100; t++) {
        int fd = socket(AF_UNIX, SOCK_SEQPACKET | SOCK_CLOEXEC, 0);
        if (fd < 0) return -1;
        int snd = 256 * 1024;
        setsockopt(fd, SOL_SOCKET, SO_SNDBUF, &snd, sizeof(snd));
        struct sockaddr_un a = {0};
        a.sun_family = AF_UNIX;
        strncpy(a.sun_path, path, sizeof(a.sun_path) - 1);
        if (connect(fd, (struct sockaddr *)&a, sizeof(a)) == 0) return fd;
        close(fd);
        struct timespec ts = {0, 100 * 1000 * 1000};
        nanosleep(&ts, NULL);
    }
    return -1;
}

int main(void) {
    signal(SIGPIPE, SIG_IGN);

    const char *paths[] = {"/sockets/api1.sock", "/sockets/api2.sock"};
    int nb = 2;
    worker_t workers[MAX_WORKERS];
    for (int i = 0; i < nb; i++) {
        int fd = connect_worker(paths[i]);
        if (fd < 0) {
            fprintf(stderr, "[lb] cannot reach %s\n", paths[i]);
            return 3;
        }
        init_worker(&workers[i], fd);
    }

    int lfd = socket(AF_INET, SOCK_STREAM | SOCK_NONBLOCK | SOCK_CLOEXEC, 0);
    int on = 1;
    setsockopt(lfd, SOL_SOCKET, SO_REUSEADDR, &on, sizeof(on));
    setsockopt(lfd, IPPROTO_TCP, TCP_DEFER_ACCEPT, &on, sizeof(on));
    struct sockaddr_in addr = {0};
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = htonl(INADDR_ANY);
    addr.sin_port = htons(9999);
    if (bind(lfd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        perror("bind");
        return 6;
    }
    if (listen(lfd, 4096) < 0) {
        perror("listen");
        return 7;
    }

    int rr = 0;
    for (;;) {
        int accepted = 0;
        while (accepted < ACCEPT_BATCH) {
            int cfd = accept4(lfd, NULL, NULL, SOCK_NONBLOCK | SOCK_CLOEXEC);
            if (cfd < 0) {
                if (errno == EINTR) continue;
                break;
            }
            accepted++;
            int one = 1;
            setsockopt(cfd, IPPROTO_TCP, TCP_NODELAY, &one, sizeof(one));
            setsockopt(cfd, IPPROTO_TCP, TCP_QUICKACK, &one, sizeof(one));

            int first = rr;
            rr = (rr + 1) % nb;
            int ok = 0;
            for (int off = 0; off < nb; off++) {
                if (send_fd(&workers[(first + off) % nb], cfd, MSG_DONTWAIT) == 0) {
                    ok = 1;
                    break;
                }
            }
            if (!ok) send_fd(&workers[first], cfd, 0);
            close(cfd);
        }
        if (accepted == 0) {
            struct pollfd pfd = {.fd = lfd, .events = POLLIN, .revents = 0};
            poll(&pfd, 1, -1);
        }
    }
}
