package types

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

var _ quic.OOBCapablePacketConn = (*webTransportListenConn)(nil)

func httpServerTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := fixture.TLS.Certificates[0]
	fixture.Close()
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	for path, block := range map[string]*pem.Block{
		certPath: {Type: "CERTIFICATE", Bytes: certificate.Certificate[0]},
		keyPath:  {Type: "PRIVATE KEY", Bytes: key},
	} {
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return certPath, keyPath
}

func httpServerTestAddress(t *testing.T) string {
	t.Helper()
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tcp.Close() }()
	addr := tcp.Addr().String()
	udp, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = udp.Close()
	return addr
}

func httpServerTestClient(t *testing.T, useQUIC bool) *http.Client {
	t.Helper()
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	if !useQUIC {
		transport := &http.Transport{TLSClientConfig: tlsConfig}
		t.Cleanup(transport.CloseIdleConnections)
		return &http.Client{Transport: transport, Timeout: 3 * time.Second}
	}
	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	quicTransport := &quic.Transport{Conn: packetConn}
	transport := &http3.Transport{
		TLSClientConfig: tlsConfig,
		EnableDatagrams: true,
		QUICConfig:      &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: true},
		Dial: func(ctx context.Context, addr string, tlsConfig *tls.Config, config *quic.Config) (*quic.Conn, error) {
			remote, err := net.ResolveUDPAddr("udp", addr)
			if err != nil {
				return nil, err
			}
			return quicTransport.DialEarly(ctx, remote, tlsConfig, config)
		},
	}
	t.Cleanup(func() {
		_ = transport.Close()
		_ = quicTransport.Close()
		_ = packetConn.Close()
	})
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}
}

func startHTTPServerTest(kind string, server *HttpServer, addr, cert, key string, ready Callable) {
	switch kind {
	case "TLS":
		server.ListenTLS(addr, cert, key, ready)
	case "HTTP3":
		server.ListenHTTP3TLS(addr, cert, key, nil, ready)
	case "WebTransport":
		server.ListenWebTransportTLS(addr, cert, key, nil, ready)
	}
}

func waitHTTPServerStopped(t *testing.T, kind, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		stack := make([]byte, 1<<18)
		stack = stack[:runtime.Stack(stack, true)]
		if !strings.Contains(string(stack), "(*HttpServer).Listen"+kind+".func") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s serving goroutine did not exit:\n%s", kind, stack)
		}
		time.Sleep(time.Millisecond)
	}
	if kind != "WebTransportTLS" {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("TCP listener was not released: %v", err)
		}
		_ = listener.Close()
	}
	if kind != "TLS" {
		listener, err := net.ListenPacket("udp", addr)
		if err != nil {
			t.Fatalf("UDP listener was not released: %v", err)
		}
		_ = listener.Close()
	}
}

func TestHttpServerTLSVariantsReadyAndClose(t *testing.T) {
	cert, key := httpServerTestCertificate(t)
	for _, kind := range []string{"TLS", "HTTP3", "WebTransport"} {
		t.Run(kind, func(t *testing.T) {
			addr := httpServerTestAddress(t)
			server := NewWebServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ready") }))
			t.Cleanup(func() { _ = server.Close(nil) })
			clients := []*http.Client{httpServerTestClient(t, kind != "TLS")}
			if kind == "HTTP3" {
				clients = append(clients, httpServerTestClient(t, false))
			}
			notifications := 0
			checkReady := func() {
				notifications++
				var requests sync.WaitGroup
				for _, client := range clients {
					requests.Go(func() {
						resp, err := client.Get("https://" + addr)
						if err != nil {
							t.Errorf("notification arrived before serving was ready: %v", err)
							return
						}
						body, err := io.ReadAll(resp.Body)
						_ = resp.Body.Close()
						if err != nil || string(body) != "ready" {
							t.Errorf("body=%q err=%v", body, err)
						}
					})
				}
				requests.Wait()
			}
			_ = server.On("listening", func(...any) { checkReady() })
			startHTTPServerTest(kind, server, addr, cert, key, checkReady)
			if notifications != 2 {
				t.Fatalf("notifications=%d, want 2", notifications)
			}
			if err := server.Close(nil); err != nil {
				t.Fatal(err)
			}
			if kind != "TLS" {
				kind += "TLS"
			}
			waitHTTPServerStopped(t, kind, addr)
		})
	}
}

func TestHttpServerTLSVariantsStartupFailure(t *testing.T) {
	cert, key := httpServerTestCertificate(t)
	for _, kind := range []string{"TLS", "HTTP3", "WebTransport"} {
		for _, failure := range []string{"certificate", "bind"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				addr := httpServerTestAddress(t)
				certificate := cert
				if failure == "certificate" {
					certificate = filepath.Join(t.TempDir(), "missing.pem")
				} else if kind == "WebTransport" {
					occupied, err := net.ListenPacket("udp", addr)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = occupied.Close() }()
				} else {
					occupied, err := net.Listen("tcp", addr)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = occupied.Close() }()
				}
				server := NewWebServer(nil)
				t.Cleanup(func() { _ = server.Close(nil) })
				notified := false
				_ = server.On("listening", func(...any) { notified = true })
				var caught any
				func() {
					defer func() { caught = recover() }()
					startHTTPServerTest(kind, server, addr, certificate, key, func() { notified = true })
				}()
				err, ok := caught.(error)
				if !ok || notified || len(server.shutdowns) != 0 {
					t.Fatalf("panic=%v notified=%v registered=%d", caught, notified, len(server.shutdowns))
				}
				if failure == "certificate" && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("certificate error identity was lost: %v", err)
				}
				if failure == "bind" {
					if _, isOpError := errors.AsType[*net.OpError](err); !isOpError {
						t.Fatalf("bind error identity was lost: %v", err)
					}
				}
				if kind == "HTTP3" {
					listener, err := net.ListenPacket("udp", addr)
					if err != nil {
						t.Fatalf("failed startup leaked the UDP listener: %v", err)
					}
					_ = listener.Close()
				}
			})
		}
	}
}

func TestHttpServerTLSVariantsCloseFromReadyCallback(t *testing.T) {
	cert, key := httpServerTestCertificate(t)
	for _, kind := range []string{"TLS", "HTTP3", "WebTransport"} {
		t.Run(kind, func(t *testing.T) {
			addr := httpServerTestAddress(t)
			server := NewWebServer(http.NotFoundHandler())
			startHTTPServerTest(kind, server, addr, cert, key, func() {
				if err := server.Close(nil); err != nil {
					t.Error(err)
				}
			})
			if kind != "TLS" {
				kind += "TLS"
			}
			waitHTTPServerStopped(t, kind, addr)
		})
	}
}

func TestHttpServerWebTransportStartupErrorBeforeSocketUse(t *testing.T) {
	cert, key := httpServerTestCertificate(t)
	addr := httpServerTestAddress(t)
	server := NewWebServer(nil)
	notified := false
	_ = server.On("listening", func(...any) { notified = true })
	var caught any
	func() {
		defer func() { caught = recover() }()
		server.ListenWebTransportTLS(addr, cert, key, &quic.Config{Versions: []quic.Version{0xdeadbeef}}, func() { notified = true })
	}()
	if caught == nil || notified || len(server.shutdowns) != 0 {
		t.Fatalf("panic=%v notified=%v registered=%d", caught, notified, len(server.shutdowns))
	}
	listener, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("failed startup leaked the UDP listener: %v", err)
	}
	_ = listener.Close()
}

func TestHttpServerHTTP3ClosePreservesHTTPSResponse(t *testing.T) {
	cert, key := httpServerTestCertificate(t)
	addr := httpServerTestAddress(t)
	entered, release := make(chan struct{}), make(chan struct{})
	server := NewWebServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "completed")
	}))
	defer func() { _ = server.Close(nil) }()
	h3 := server.ListenHTTP3TLS(addr, cert, key, nil, nil)
	client := httpServerTestClient(t, false)
	result := make(chan error, 1)
	go func() {
		resp, err := client.Get("https://" + addr)
		if err == nil {
			var body []byte
			body, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err == nil && string(body) != "completed" {
				err = errors.New("incomplete response")
			}
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("HTTPS handler did not start")
	}
	if err := h3.Close(); err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case err := <-result:
		close(release)
		t.Fatalf("HTTP3 close interrupted the HTTPS handler: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	waitHTTPServerStopped(t, "HTTP3TLS", addr)
}

func TestHttpServerCloseAllowsHandlerToRegisterListener(t *testing.T) {
	entered, proceed, registered := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var server *HttpServer
	server = NewWebServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-proceed
		server.Listen("127.0.0.1:0", nil)
		close(registered)
		_, _ = io.WriteString(w, "completed")
	}))
	t.Cleanup(func() { _ = server.Close(nil) })
	addr := httpServerTestAddress(t)
	original := server.Listen(addr, nil)
	shuttingDown := make(chan struct{}, 1)
	original.RegisterOnShutdown(func() {
		select {
		case shuttingDown <- struct{}{}:
		default:
		}
	})
	client := &http.Client{Timeout: 3 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	requestDone := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + addr)
		if err == nil {
			_, err = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		requestDone <- err
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- server.Close(nil) }()
	<-shuttingDown
	close(proceed)
	select {
	case <-registered:
	case <-time.After(time.Second):
		// Release the request even when the old registry lock blocks its handler.
		_ = original.Close()
		<-closed
		<-registered
		t.Fatal("Close held the registry lock while waiting for the request")
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
}
