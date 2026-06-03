package server

import (
	"errors"

	"golang.org/x/sys/unix"
)

const maxFDsPerMsg = 64

type conn struct {
	fd int
	rb []byte
	wb []byte
	wn int
}

type Server struct {
	epfd  int
	conns map[int]*conn
	h     *Handler
	buf   []byte
}

func New(h *Handler) (*Server, error) {
	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return &Server{epfd: epfd, conns: make(map[int]*conn, 4096), h: h, buf: make([]byte, 64*1024)}, nil
}

func (s *Server) add(fd int) {
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return
	}
	ev := unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLET, Fd: int32(fd)}
	if err := unix.EpollCtl(s.epfd, unix.EPOLL_CTL_ADD, fd, &ev); err != nil {
		unix.Close(fd)
		return
	}
	s.conns[fd] = &conn{fd: fd, rb: make([]byte, 0, 4096), wb: make([]byte, 0, 256)}
}

func (s *Server) drop(fd int) {
	unix.EpollCtl(s.epfd, unix.EPOLL_CTL_DEL, fd, nil)
	unix.Close(fd)
	delete(s.conns, fd)
}

func (s *Server) readDrain(c *conn) bool {
	for {
		n, err := unix.Read(c.fd, s.buf)
		if n > 0 {
			c.rb = append(c.rb, s.buf[:n]...)
			if !s.h.handle(c) {
				return false
			}
			if len(c.wb) > c.wn && !s.flush(c) {
				return false
			}
		}
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			return true
		}
		if n == 0 || err != nil {
			return false
		}
	}
}

func (s *Server) flush(c *conn) bool {
	for c.wn < len(c.wb) {
		n, err := unix.Write(c.fd, c.wb[c.wn:])
		if n > 0 {
			c.wn += n
		}
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			ev := unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLOUT | unix.EPOLLRDHUP | unix.EPOLLET, Fd: int32(c.fd)}
			unix.EpollCtl(s.epfd, unix.EPOLL_CTL_MOD, c.fd, &ev)
			return true
		}
		if err != nil {
			return false
		}
	}
	c.wb, c.wn = c.wb[:0], 0
	ev := unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLET, Fd: int32(c.fd)}
	unix.EpollCtl(s.epfd, unix.EPOLL_CTL_MOD, c.fd, &ev)
	return true
}

func (s *Server) Run(specialFD int, listener bool) error {
	ev := unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLET, Fd: int32(specialFD)}
	if err := unix.EpollCtl(s.epfd, unix.EPOLL_CTL_ADD, specialFD, &ev); err != nil {
		return err
	}
	events := make([]unix.EpollEvent, 256)
	for {
		n, err := unix.EpollWait(s.epfd, events, -1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			fd, e := int(events[i].Fd), events[i].Events
			if fd == specialFD {
				if listener {
					s.acceptLoop(specialFD)
				} else {
					s.recvLoop(specialFD)
				}
				continue
			}
			c := s.conns[fd]
			if c == nil {
				continue
			}
			if e&(unix.EPOLLHUP|unix.EPOLLERR) != 0 {
				s.drop(fd)
				continue
			}
			if e&unix.EPOLLOUT != 0 && !s.flush(c) {
				s.drop(fd)
				continue
			}
			if e&unix.EPOLLIN != 0 && !s.readDrain(c) {
				s.drop(fd)
				continue
			}
			if e&unix.EPOLLRDHUP != 0 && c.wn == len(c.wb) {
				s.drop(fd)
			}
		}
	}
}

func (s *Server) acceptLoop(lfd int) {
	for {
		nfd, _, err := unix.Accept4(lfd, unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC)
		if err != nil {
			return
		}
		s.add(nfd)
	}
}

func (s *Server) recvLoop(ctlFD int) {
	for {
		fds, err := recvFDs(ctlFD)
		if err != nil {
			return
		}
		for _, fd := range fds {
			s.add(fd)
		}
	}
}

func recvFDs(ctlFD int) ([]int, error) {
	oob := make([]byte, unix.CmsgSpace(maxFDsPerMsg*4))
	dummy := make([]byte, 1)
	n, oobn, recvflags, _, err := unix.Recvmsg(ctlFD, dummy, oob, 0)
	if err != nil {
		return nil, err
	}
	if n == 0 && oobn == 0 {
		return nil, errors.New("control socket closed")
	}
	if recvflags&unix.MSG_CTRUNC != 0 {
		return nil, errors.New("truncated control message")
	}
	scms, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, err
	}
	var fds []int
	for i := range scms {
		if scms[i].Header.Level != unix.SOL_SOCKET || scms[i].Header.Type != unix.SCM_RIGHTS {
			continue
		}
		r, err := unix.ParseUnixRights(&scms[i])
		if err != nil {
			return fds, err
		}
		fds = append(fds, r...)
	}
	return fds, nil
}
