package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHServerAdvertisesConfiguredVersion(t *testing.T) {
	t.Parallel()

	serverConfig := newSSHServerConfig(newTestSigner(t))
	got, err := fetchServerVersion(serverConfig)
	if err != nil {
		t.Fatalf("fetch server version: %v", err)
	}

	if got != sshServerVersion {
		t.Fatalf("server version = %q, want %q", got, sshServerVersion)
	}
}

func TestSSHServerAcceptsCommonAuthMethods(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		allowNone  bool
		auth       func(t *testing.T) ssh.AuthMethod
		wantMethod string
	}{
		{
			name:       "none",
			allowNone:  true,
			wantMethod: "none",
		},
		{
			name:      "password",
			allowNone: false,
			auth: func(t *testing.T) ssh.AuthMethod {
				t.Helper()
				return ssh.Password("anything")
			},
			wantMethod: "password",
		},
		{
			name:      "publickey",
			allowNone: false,
			auth: func(t *testing.T) ssh.AuthMethod {
				t.Helper()
				return ssh.PublicKeys(newTestSigner(t))
			},
			wantMethod: "publickey",
		},
		{
			name:      "keyboard-interactive",
			allowNone: false,
			auth: func(t *testing.T) ssh.AuthMethod {
				t.Helper()
				return ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
					return make([]string, len(questions)), nil
				})
			},
			wantMethod: "keyboard-interactive",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			serverConfig := newSSHServerConfig(newTestSigner(t))
			serverConfig.NoClientAuth = tt.allowNone

			var (
				mu              sync.Mutex
				acceptedMethods []string
			)
			serverConfig.AuthLogCallback = func(conn ssh.ConnMetadata, method string, err error) {
				if err != nil {
					return
				}

				mu.Lock()
				acceptedMethods = append(acceptedMethods, method)
				mu.Unlock()
			}

			clientConfig := &ssh.ClientConfig{
				User:            "tester",
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			}
			if tt.auth != nil {
				clientConfig.Auth = []ssh.AuthMethod{tt.auth(t)}
			}

			if err := runTestHandshake(serverConfig, clientConfig); err != nil {
				t.Fatalf("handshake failed: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(acceptedMethods) == 0 {
				t.Fatalf("no successful auth method was recorded")
			}
			if got := acceptedMethods[len(acceptedMethods)-1]; got != tt.wantMethod {
				t.Fatalf("accepted method = %q, want %q", got, tt.wantMethod)
			}
		})
	}
}

func runTestHandshake(serverConfig *ssh.ServerConfig, clientConfig *ssh.ClientConfig) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		serverNetConn, err := listener.Accept()
		if err != nil {
			serverErrCh <- fmt.Errorf("accept: %w", err)
			return
		}
		defer serverNetConn.Close()

		conn, chans, reqs, err := ssh.NewServerConn(serverNetConn, serverConfig)
		if err == nil {
			go ssh.DiscardRequests(reqs)
			go discardTestChannels(chans)
			_ = conn.Close()
		}
		serverErrCh <- err
	}()

	clientNetConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer clientNetConn.Close()

	clientConn, _, _, clientErr := ssh.NewClientConn(clientNetConn, listener.Addr().String(), clientConfig)
	if clientErr == nil {
		_ = clientConn.Close()
	}

	serverErr := <-serverErrCh
	if clientErr != nil {
		return fmt.Errorf("client: %w", clientErr)
	}
	if serverErr != nil {
		return fmt.Errorf("server: %w", serverErr)
	}

	return nil
}

func fetchServerVersion(serverConfig *ssh.ServerConfig) (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		serverNetConn, err := listener.Accept()
		if err != nil {
			serverErrCh <- fmt.Errorf("accept: %w", err)
			return
		}
		defer serverNetConn.Close()

		conn, chans, reqs, err := ssh.NewServerConn(serverNetConn, serverConfig)
		if err == nil {
			go ssh.DiscardRequests(reqs)
			go discardTestChannels(chans)
			_ = conn.Close()
		}
		serverErrCh <- err
	}()

	clientNetConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		return "", fmt.Errorf("dial: %w", err)
	}
	defer clientNetConn.Close()

	clientConn, _, _, clientErr := ssh.NewClientConn(clientNetConn, listener.Addr().String(), &ssh.ClientConfig{
		User:            "tester",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	serverErr := <-serverErrCh
	if clientErr != nil {
		return "", fmt.Errorf("client: %w", clientErr)
	}
	defer clientConn.Close()
	if serverErr != nil {
		return "", fmt.Errorf("server: %w", serverErr)
	}

	return string(clientConn.ServerVersion()), nil
}

func discardTestChannels(chans <-chan ssh.NewChannel) {
	for newChan := range chans {
		_ = newChan.Reject(ssh.Prohibited, "test session not available")
	}
}

func newTestSigner(t *testing.T) ssh.Signer {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	return signer
}
