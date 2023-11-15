package util

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

func unixDialer(ctx context.Context, addr string) (net.Conn, error) {
	dialer := net.Dialer{}
	return dialer.DialContext(ctx, "unix", addr)
}

// Connect is a helper function to initiate a grpc client connection to server running at endpoint using tlsConfig
func Connect(endpoint string, tlsConfig *tls.Config, dialOptions ...grpc.DialOption) (*grpc.ClientConn, error) {
	proto, address, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	dialOptions = append(dialOptions, grpc.WithBackoffMaxDelay(time.Second))
	if tlsConfig != nil {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}

	if proto == "unix" {
		dialOptions = append(dialOptions, grpc.WithContextDialer(unixDialer))
	}
	// This is necessary when connecting via TCP and does not hurt
	// when using Unix domain sockets. It ensures that gRPC detects a dead connection
	// in a timely manner.
	// Code lifted from https://github.com/kubernetes-csi/csi-test/commit/6b8830bf5959a1c51c6e98fe514b22818b51eeeb
	dialOptions = append(dialOptions, grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 30 * time.Second, PermitWithoutStream: true}))

	return grpc.Dial(address, dialOptions...)
}

// NewServer is a helper function to start a grpc server at the given endpoint.
func NewServer(endpoint string, tlsConfig *tls.Config, opts ...grpc.ServerOption) (*grpc.Server, net.Listener, error) {
	proto, addr, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, nil, err
	}

	if proto == "unix" {
		if err = os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return nil, nil, err
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		return nil, nil, err
	}

	if tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	return grpc.NewServer(opts...), listener, nil
}

// ServerTLS prepares the TLS configuration needed for a server with given
// encoded certficate and private key.
func ServerTLS(caCert, cert, key []byte, peerName string) (*tls.Config, error) {
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("failed to append CA certificate to pool")
	}

	certificate, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}

	return serverConfig(certPool, &certificate, peerName), nil
}

// LoadServerTLS prepares the TLS configuration needed for a server with the given certificate files.
// peerName is either the name that the client is expected to have a certificate for or empty,
// in which case any client is allowed to connect.
func LoadServerTLS(caFile, certFile, keyFile, peerName string) (*tls.Config, error) {
	certPool, peerCert, err := loadCertificate(caFile, certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return serverConfig(certPool, peerCert, peerName), nil
}

func serverConfig(certPool *x509.CertPool, peerCert *tls.Certificate, peerName string) *tls.Config {
	return &tls.Config{
		GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			if info == nil {
				return nil, errors.New("nil client info passed")
			}

			config := &tls.Config{
				MinVersion:    tls.VersionTLS13,
				Renegotiation: tls.RenegotiateNever,
				Certificates:  []tls.Certificate{*peerCert},
				ClientCAs:     certPool,
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					// Common name check when accepting a connection from a client.
					if peerName == "" {
						// All names allowed.
						return nil
					}

					if len(verifiedChains) == 0 ||
						len(verifiedChains[0]) == 0 {
						return errors.New("no valid certificate")
					}

					for _, name := range verifiedChains[0][0].DNSNames {
						if name == peerName {
							return nil
						}
					}
					// For compatibility - using CN as hostName
					commonName := verifiedChains[0][0].Subject.CommonName
					if commonName == peerName {
						return nil
					}
					return fmt.Errorf("certificate is not signed for %q hostname", peerName)
				},
			}
			if peerName != "" {
				config.ClientAuth = tls.RequireAndVerifyClientCert
			}
			return config, nil
		},
	}
}

// ClientTLS prepares the TLS configuration that can be used by a client while connecting to a server
// with given encoded certificate and private key.
// peerName must be provided when expecting the server to offer a certificate with that CommonName.
func ClientTLS(caCert, cert, key []byte, peerName string) (*tls.Config, error) {
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("failed to append CA certificate to pool")
	}

	certificate, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}

	return clientConfig(certPool, &certificate, peerName), nil
}

// LoadClientTLS prepares the TLS configuration that can be used by a client while connecting to a server.
// peerName must be provided when expecting the server to offer a certificate with that CommonName. caFile, certFile, and keyFile are all optional.
func LoadClientTLS(caFile, certFile, keyFile, peerName string) (*tls.Config, error) {
	certPool, peerCert, err := loadCertificate(caFile, certFile, keyFile)
	if err != nil {
		return nil, err
	}

	return clientConfig(certPool, peerCert, peerName), nil
}

func clientConfig(certPool *x509.CertPool, peerCert *tls.Certificate, peerName string) *tls.Config {
	tlsConfig := &tls.Config{
		MinVersion:    tls.VersionTLS13,
		Renegotiation: tls.RenegotiateNever,
		ServerName:    peerName,
		RootCAs:       certPool,
	}
	if peerCert != nil {
		tlsConfig.Certificates = append(tlsConfig.Certificates, *peerCert)
	}
	return tlsConfig
}

func loadCertificate(caFile, certFile, keyFile string) (certPool *x509.CertPool, peerCert *tls.Certificate, err error) {
	if certFile != "" || keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, nil, err
		}
		peerCert = &cert
	}

	if caFile != "" {
		caCert, err := ioutil.ReadFile(caFile)
		if err != nil {
			return nil, nil, err
		}

		certPool = x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM(caCert); !ok {
			return nil, nil, fmt.Errorf("failed to append certs from %s", caFile)
		}
	}

	return
}

// parseEndpoint splits ep string into proto and address
// the supported protocols are "tcp" and "unix" unsupported protocols will return error
// if the ep (url) does not contain a protocol, we return "tcp" for backwards compatibility
func parseEndpoint(ep string) (proto string, address string, err error) {
	s := strings.SplitN(ep, "://", 2)
	if len(s) == 0 {
		return "", "", fmt.Errorf("invalid endpoint: %v", ep)
	}

	if len(s) > 1 && s[1] != "" {
		if s[0] != "unix" && s[0] != "tcp" {
			return "", "", fmt.Errorf("invalid endpoint: %v unsupported proto: %v", ep, s[0])
		}
		return s[0], s[1], nil
	}

	// default protocol tcp to be backwards compatible
	return "tcp", s[0], nil
}
