package adapter

import (
	"crypto/tls"
	"crypto/x509"
	"example.com/goml-ecosystem/web/testcert"
	"net"
	"testing"
	"time"
)

func TestTLSListenerEchoAndClose(t *testing.T) {
	certificate, key, err := testcert.Generate()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := Listen("127.0.0.1:0", certificate, key, tls.VersionTLS13)
	if err != nil {
		t.Fatal(err)
	}
	defer ListenerClose(listener)
	done := make(chan error, 1)
	go func() {
		stream, _, err := Accept(listener)
		if err != nil {
			done <- err
			return
		}
		defer Close(stream)
		if err = Handshake(stream, int64(time.Second)); err != nil {
			done <- err
			return
		}
		data := make([]byte, 4)
		n, err := Read(stream, data, int64(time.Second))
		if err == nil {
			_, err = Write(stream, data[:n], int64(time.Second))
		}
		done <- err
	}()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		t.Fatal("certificate")
	}
	client, err := tls.Dial("tcp", ListenerAddress(listener), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13, NextProtos: []string{"h2", "http/1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.ConnectionState().NegotiatedProtocol != "http/1.1" {
		t.Fatal("ALPN")
	}
	if _, err = client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err = client.Read(response); err != nil {
		t.Fatal(err)
	}
	if string(response) != "ping" {
		t.Fatal(string(response))
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTLSHandshakeDeadlineAndListenerCancellation(t *testing.T) {
	certificate, key, err := testcert.Generate()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := Listen("127.0.0.1:0", certificate, key, tls.VersionTLS12)
	if err != nil {
		t.Fatal(err)
	}
	defer ListenerClose(listener)
	client, err := net.Dial("tcp", ListenerAddress(listener))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	stream, _, err := Accept(listener)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(stream)
	if err = Handshake(stream, int64(20*time.Millisecond)); !IsTimeout(err) {
		t.Fatalf("handshake: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, _, err := Accept(listener); done <- err }()
	if err = ListenerClose(listener); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("accept succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("accept did not unblock")
	}
}
