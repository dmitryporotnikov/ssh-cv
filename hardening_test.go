package main

import (
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// A successfully authenticated client can withhold its first session channel.
// Such clients must release their slots without relying on client disconnects.
func TestAuthenticatedConnectionWithoutSessionTimesOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	limiter := make(chan struct{}, 1)
	limiter <- struct{}{}
	done := make(chan struct{})
	cfg := &Config{Server: ServerConfig{HandshakeTimeoutSeconds: 2, SessionTimeoutSeconds: 1}}
	server := newSSHServerConfig(newTestSigner(t))
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			handleConnection(conn, server, cfg, limiter)
		}
		close(done)
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User: "idle-client", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case <-done:
		if len(limiter) != 0 {
			t.Fatal("connection slot was not released")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("authenticated connection without session did not time out")
	}
}
