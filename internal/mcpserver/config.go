package mcpserver

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

const ProtocolVersion = "2025-11-25"

type Config struct {
	AuthToken                   string
	RemoteAgent                 string
	RemoteToken                 string
	RemoteTLS                   bool
	RemoteTLSCA                 string
	RemoteTLSServerName         string
	RemoteTLSInsecureSkipVerify bool
	RemoteTLSClientCert         string
	RemoteTLSClientKey          string
	EnableWrite                 bool
	MaxEventsReturned           int
	MaxLogLinesReturned         int
	DefaultToolTimeoutSeconds   int
}

func ConfigFromEnv() Config {
	return Config{
		AuthToken:                   firstNonEmpty(os.Getenv("TORQUE_MCP_TOKEN"), os.Getenv("TORQUE_MCP_AUTH_TOKEN")),
		RemoteAgent:                 firstNonEmpty(os.Getenv("TORQUE_MCP_REMOTE_AGENT"), os.Getenv("TORQUE_REMOTE_AGENT")),
		RemoteToken:                 os.Getenv("TORQUE_REMOTE_TOKEN"),
		RemoteTLS:                   envBool("TORQUE_REMOTE_TLS"),
		RemoteTLSCA:                 os.Getenv("TORQUE_REMOTE_TLS_CA"),
		RemoteTLSServerName:         os.Getenv("TORQUE_REMOTE_TLS_SERVER_NAME"),
		RemoteTLSInsecureSkipVerify: envBool("TORQUE_REMOTE_TLS_INSECURE_SKIP_VERIFY"),
		RemoteTLSClientCert:         os.Getenv("TORQUE_REMOTE_TLS_CLIENT_CERT"),
		RemoteTLSClientKey:          os.Getenv("TORQUE_REMOTE_TLS_CLIENT_KEY"),
		EnableWrite:                 envBool("TORQUE_MCP_ENABLE_WRITE"),
		MaxEventsReturned:           500,
		MaxLogLinesReturned:         500,
		DefaultToolTimeoutSeconds:   300,
	}
}

func (c Config) withDefaults() Config {
	if c.MaxEventsReturned <= 0 {
		c.MaxEventsReturned = 500
	}
	if c.MaxLogLinesReturned <= 0 {
		c.MaxLogLinesReturned = 500
	}
	if c.DefaultToolTimeoutSeconds <= 0 {
		c.DefaultToolTimeoutSeconds = 300
	}
	if strings.TrimSpace(c.RemoteToken) == "" {
		c.RemoteToken = strings.TrimSpace(os.Getenv("TORQUE_REMOTE_TOKEN"))
	}
	if strings.TrimSpace(c.AuthToken) == "" {
		c.AuthToken = firstNonEmpty(os.Getenv("TORQUE_MCP_TOKEN"), os.Getenv("TORQUE_MCP_AUTH_TOKEN"))
	}
	return c
}

func (c Config) remoteCredentials() (credentials.TransportCredentials, error) {
	if !c.RemoteTLS && strings.TrimSpace(c.RemoteTLSCA) == "" && strings.TrimSpace(c.RemoteTLSServerName) == "" &&
		!c.RemoteTLSInsecureSkipVerify && strings.TrimSpace(c.RemoteTLSClientCert) == "" && strings.TrimSpace(c.RemoteTLSClientKey) == "" {
		return insecure.NewCredentials(), nil
	}
	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: c.RemoteTLSInsecureSkipVerify,
	}
	if strings.TrimSpace(c.RemoteTLSClientCert) != "" || strings.TrimSpace(c.RemoteTLSClientKey) != "" {
		if strings.TrimSpace(c.RemoteTLSClientCert) == "" || strings.TrimSpace(c.RemoteTLSClientKey) == "" {
			return nil, fmt.Errorf("both remote TLS client cert and key are required for mTLS")
		}
		cert, err := tls.LoadX509KeyPair(strings.TrimSpace(c.RemoteTLSClientCert), strings.TrimSpace(c.RemoteTLSClientKey))
		if err != nil {
			return nil, fmt.Errorf("load remote TLS client keypair: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	serverName := strings.TrimSpace(c.RemoteTLSServerName)
	if serverName == "" {
		if host, _, err := net.SplitHostPort(strings.TrimSpace(c.RemoteAgent)); err == nil {
			serverName = host
		}
	}
	if serverName != "" {
		tlsCfg.ServerName = serverName
	}
	if !c.RemoteTLSInsecureSkipVerify {
		pool, _ := x509.SystemCertPool()
		if pool == nil {
			pool = x509.NewCertPool()
		}
		if ca := strings.TrimSpace(c.RemoteTLSCA); ca != "" {
			raw, err := os.ReadFile(ca)
			if err != nil {
				return nil, fmt.Errorf("read remote TLS CA: %w", err)
			}
			if !pool.AppendCertsFromPEM(raw) {
				return nil, fmt.Errorf("parse remote TLS CA PEM from %q", ca)
			}
		}
		tlsCfg.RootCAs = pool
	}
	return credentials.NewTLS(tlsCfg), nil
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
