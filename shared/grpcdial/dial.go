package grpcdial

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TransportCredentials returns insecure credentials for non-production environments
// and TLS credentials for production dials.
func TransportCredentials(appEnv, target string) (credentials.TransportCredentials, error) {
	if strings.TrimSpace(appEnv) == "" {
		appEnv = strings.TrimSpace(os.Getenv("APP_ENV"))
	}

	if !isProduction(appEnv) {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile := strings.TrimSpace(os.Getenv("GRPC_TLS_CA_FILE")); caFile != "" {
		caBytes, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read grpc ca file: %w", err)
		}

		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caBytes); !ok {
			return nil, fmt.Errorf("parse grpc ca file %q", caFile)
		}
		tlsConfig.RootCAs = pool
	}

	serverName := strings.TrimSpace(os.Getenv("GRPC_TLS_SERVER_NAME"))
	if serverName == "" {
		if host, _, err := net.SplitHostPort(target); err == nil {
			serverName = host
		} else {
			serverName = strings.TrimSuffix(target, ":443")
		}
	}
	if serverName != "" {
		tlsConfig.ServerName = serverName
	}

	return credentials.NewTLS(tlsConfig), nil
}

func isProduction(appEnv string) bool {
	switch strings.ToLower(strings.TrimSpace(appEnv)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}