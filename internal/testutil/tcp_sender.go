package testutil

import (
	"bufio"
	"net"
)

type TCPSender struct {
	conn net.Conn
	w    *bufio.Writer
}

func NewTCPSender(addr string) (*TCPSender, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &TCPSender{conn: conn, w: bufio.NewWriter(conn)}, nil
}

func (s *TCPSender) SendLine(line string) error {
	_, err := s.w.WriteString(line + "\n")
	if err != nil {
		return err
	}
	return s.w.Flush()
}

func (s *TCPSender) Close() error {
	return s.conn.Close()
}
