package server

import (
	"os"

	"golang.org/x/sys/unix"
)

func TCPListen(port int) (int, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	if err := unix.Bind(fd, &unix.SockaddrInet4{Port: port}); err != nil {
		unix.Close(fd)
		return -1, err
	}
	if err := unix.Listen(fd, 4096); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func SeqpacketListen(path string) (int, error) {
	os.Remove(path)
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
		unix.Close(fd)
		return -1, err
	}
	if err := unix.Listen(fd, 16); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func AcceptControl(lfd int) (int, error) {
	cfd, _, err := unix.Accept4(lfd, unix.SOCK_CLOEXEC)
	if err != nil {
		return -1, err
	}
	if err := unix.SetNonblock(cfd, true); err != nil {
		unix.Close(cfd)
		return -1, err
	}
	return cfd, nil
}
