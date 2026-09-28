package adapter

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

type Listener struct {
	listener net.Listener
	config   *tls.Config
}

type Stream struct {
	raw  net.Conn
	conn *tls.Conn
	once sync.Once
	err  error
}

type ListenerHandle = *Listener
type StreamHandle = *Stream

func Listen(address string, certificate, key []byte, minimum uint16) (*Listener, error) {
	if minimum != tls.VersionTLS12 && minimum != tls.VersionTLS13 {
		return nil, errors.New("TLS minimum version must be 1.2 or 1.3")
	}
	pair, err := tls.X509KeyPair(certificate, key)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &Listener{listener, &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: minimum, NextProtos: []string{"http/1.1"}}}, nil
}

func Accept(listener *Listener) (*Stream, string, error) {
	conn, err := listener.listener.Accept()
	if err != nil {
		return nil, "", err
	}
	return &Stream{raw: conn, conn: tls.Server(conn, listener.config)}, conn.RemoteAddr().String(), nil
}

func ListenerAddress(listener *Listener) string { return listener.listener.Addr().String() }
func ListenerClose(listener *Listener) error    { return listener.listener.Close() }

func Handshake(stream *Stream, timeout int64) error {
	if timeout <= 0 {
		return context.DeadlineExceeded
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout))
	defer cancel()
	return stream.conn.HandshakeContext(ctx)
}

func deadline(timeout int64) time.Time {
	if timeout < 0 {
		return time.Time{}
	}
	return time.Now().Add(time.Duration(timeout))
}

func Read(stream *Stream, buffer []byte, timeout int64) (int, error) {
	if err := stream.conn.SetReadDeadline(deadline(timeout)); err != nil {
		return 0, err
	}
	n, err := stream.conn.Read(buffer)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return n, err
}

func Write(stream *Stream, buffer []byte, timeout int64) (int, error) {
	if err := stream.conn.SetWriteDeadline(deadline(timeout)); err != nil {
		return 0, err
	}
	return stream.conn.Write(buffer)
}

func Close(stream *Stream) error {
	stream.once.Do(func() {
		stream.err = stream.raw.Close()
		_ = stream.conn.Close()
	})
	return stream.err
}

func IsTimeout(err error) bool {
	var problem net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &problem) && problem.Timeout()
}
