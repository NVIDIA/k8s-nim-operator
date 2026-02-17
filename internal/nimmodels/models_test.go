package nimmodels

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewTLSTransport(t *testing.T) {
	t.Run("valid CA bundle", func(t *testing.T) {
		tmpDir := t.TempDir()
		caFile := filepath.Join(tmpDir, "service-ca.crt")

		caPEM := generateSelfSignedCACert(t)
		if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
			t.Fatalf("failed to write test CA file: %v", err)
		}

		transport, err := newTLSTransport(caFile)
		if err != nil {
			t.Fatalf("newTLSTransport() returned error: %v", err)
		}
		if transport == nil {
			t.Fatal("newTLSTransport() returned nil transport")
		}
		if transport.TLSClientConfig == nil {
			t.Fatal("TLSClientConfig is nil")
		}
		if transport.TLSClientConfig.RootCAs == nil {
			t.Fatal("RootCAs is nil")
		}
		if transport.TLSClientConfig.MinVersion != 0x0303 { // tls.VersionTLS12
			t.Errorf("MinVersion = %#x, want %#x (TLS 1.2)", transport.TLSClientConfig.MinVersion, 0x0303)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := newTLSTransport("/nonexistent/path/ca.crt")
		if err == nil {
			t.Fatal("newTLSTransport() should return error for missing file")
		}
	})

	t.Run("invalid PEM data", func(t *testing.T) {
		tmpDir := t.TempDir()
		caFile := filepath.Join(tmpDir, "bad-ca.crt")

		if err := os.WriteFile(caFile, []byte("not a valid PEM"), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := newTLSTransport(caFile)
		if err == nil {
			t.Fatal("newTLSTransport() should return error for invalid PEM data")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		caFile := filepath.Join(tmpDir, "empty-ca.crt")

		if err := os.WriteFile(caFile, []byte(""), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := newTLSTransport(caFile)
		if err == nil {
			t.Fatal("newTLSTransport() should return error for empty file")
		}
	})
}

func TestGetURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		uri      string
		scheme   string
		expected string
	}{
		{
			name:     "with scheme",
			endpoint: "example.com",
			uri:      "/v1/models",
			scheme:   "https",
			expected: "https://example.com/v1/models",
		},
		{
			name:     "without scheme",
			endpoint: "https://example.com",
			uri:      "/v1/models",
			scheme:   "",
			expected: "https://example.com/v1/models",
		},
		{
			name:     "http scheme",
			endpoint: "example.com:8000",
			uri:      "/v1/models",
			scheme:   "http",
			expected: "http://example.com:8000/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getURL(tt.endpoint, tt.uri, tt.scheme)
			if result != tt.expected {
				t.Errorf("getURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestReadBearerToken(t *testing.T) {
	t.Run("valid token file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "token")

		if err := os.WriteFile(tokenFile, []byte("my-sa-token-value"), 0600); err != nil {
			t.Fatalf("failed to write token file: %v", err)
		}

		token, ok := readBearerToken(tokenFile)
		if !ok {
			t.Fatal("readBearerToken() returned false, want true")
		}
		if token != "my-sa-token-value" {
			t.Errorf("readBearerToken() = %q, want %q", token, "my-sa-token-value")
		}
	})

	t.Run("token with whitespace/newline", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "token")

		if err := os.WriteFile(tokenFile, []byte("  my-token\n"), 0600); err != nil {
			t.Fatalf("failed to write token file: %v", err)
		}

		token, ok := readBearerToken(tokenFile)
		if !ok {
			t.Fatal("readBearerToken() returned false, want true")
		}
		if token != "my-token" {
			t.Errorf("readBearerToken() = %q, want %q", token, "my-token")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, ok := readBearerToken("/nonexistent/path/token")
		if ok {
			t.Fatal("readBearerToken() returned true for missing file, want false")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "token")

		if err := os.WriteFile(tokenFile, []byte(""), 0600); err != nil {
			t.Fatalf("failed to write token file: %v", err)
		}

		_, ok := readBearerToken(tokenFile)
		if ok {
			t.Fatal("readBearerToken() returned true for empty file, want false")
		}
	})

	t.Run("whitespace-only file", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "token")

		if err := os.WriteFile(tokenFile, []byte("  \n\t\n"), 0600); err != nil {
			t.Fatalf("failed to write token file: %v", err)
		}

		_, ok := readBearerToken(tokenFile)
		if ok {
			t.Fatal("readBearerToken() returned true for whitespace-only file, want false")
		}
	})
}

// generateSelfSignedCACert creates a self-signed CA certificate for testing.
func generateSelfSignedCACert(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}
