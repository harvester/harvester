package aggregation

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rancher/remotedialer"
	"github.com/rancher/steve/pkg/auth"
	"github.com/sirupsen/logrus"
)

const (
	HandshakeTimeOut = 10 * time.Second
)

func ListenAndServe(ctx context.Context, url string, caCert []byte, token string, handler http.Handler) {
	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: HandshakeTimeOut,
	}

	if caCert != nil && len(caCert) == 0 {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	} else if len(caCert) > 0 {
		if _, err := http.Get(url); err != nil {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCert)
			dialer.TLSClientConfig = &tls.Config{
				RootCAs: pool,
			}
		}
	}

	handler = auth.ToMiddleware(auth.AuthenticatorFunc(auth.Impersonation))(handler)

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+token)

	for {
		err := serve(ctx, dialer, url, headers, handler)
		if err != nil {
			logrus.Errorf("Failed to dial steve aggregation server: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func serve(ctx context.Context, dialer websocket.Dialer, url string, headers http.Header, handler http.Handler) error {
	url = strings.Replace(url, "http://", "ws://", 1)
	url = strings.Replace(url, "https://", "wss://", 1)

	// ensure we clean up everything on exit
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	conn, _, err := dialer.DialContext(dialCtx, url, headers)
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	listener := NewListener("steve")
	server := http.Server{
		Handler: handler,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	session := remotedialer.NewClientSessionWithDialer(allowAll, conn, listener.Dial)
	defer session.Close()

	_, err = session.Serve(ctx)
	return err
}

func allowAll(_, _ string) bool {
	return true
}
