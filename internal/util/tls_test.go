package util

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTLSConfig(t *testing.T) {
	// Create a temporary directory for test certificates
	tmpDir := t.TempDir()
	originalGetStateDir := getStateDir
	getStateDir = func() (string, error) { return tmpDir, nil }
	defer func() { getStateDir = originalGetStateDir }()

	t.Run("generates new certificate", func(t *testing.T) {
		config, err := TLSConfig()
		if err != nil {
			t.Fatalf("TLSConfig failed: %v", err)
		}

		if config == nil {
			t.Fatal("TLS config is nil")
		}

		if len(config.Certificates) != 1 {
			t.Fatalf("Expected 1 certificate, got %d", len(config.Certificates))
		}

		if config.ServerName != "op-authd-local" {
			t.Errorf("Expected ServerName 'op-authd-local', got %q", config.ServerName)
		}

		// Verify certificate files exist
		certPath := filepath.Join(tmpDir, "tls.crt")
		keyPath := filepath.Join(tmpDir, "tls.key")

		if _, err := os.Stat(certPath); os.IsNotExist(err) {
			t.Error("Certificate file not created")
		}

		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			t.Error("Key file not created")
		}

		// Verify file permissions
		if info, err := os.Stat(certPath); err == nil {
			if info.Mode().Perm() != 0o600 {
				t.Errorf("Certificate file has wrong permissions: %o", info.Mode().Perm())
			}
		}

		if info, err := os.Stat(keyPath); err == nil {
			if info.Mode().Perm() != 0o600 {
				t.Errorf("Key file has wrong permissions: %o", info.Mode().Perm())
			}
		}
	})

	t.Run("reuses existing valid certificate", func(t *testing.T) {
		// First call creates the certificate
		_, err := TLSConfig()
		if err != nil {
			t.Fatalf("First TLSConfig call failed: %v", err)
		}

		// Get modification time of certificate file
		certPath := filepath.Join(tmpDir, "tls.crt")
		info1, err := os.Stat(certPath)
		if err != nil {
			t.Fatalf("Failed to stat certificate: %v", err)
		}

		// Small delay to ensure different modification times if regenerated
		time.Sleep(10 * time.Millisecond)

		// Second call should reuse existing certificate
		_, err = TLSConfig()
		if err != nil {
			t.Fatalf("Second TLSConfig call failed: %v", err)
		}

		info2, err := os.Stat(certPath)
		if err != nil {
			t.Fatalf("Failed to stat certificate after second call: %v", err)
		}

		// Certificate should not have been regenerated
		if !info1.ModTime().Equal(info2.ModTime()) {
			t.Error("Certificate was regenerated when it should have been reused")
		}
	})
}

func TestClientTLSConfig(t *testing.T) {
	// Create a temporary directory for test certificates
	tmpDir := t.TempDir()
	originalGetStateDir := getStateDir
	getStateDir = func() (string, error) { return tmpDir, nil }
	defer func() { getStateDir = originalGetStateDir }()

	// First generate server certificate
	_, err := TLSConfig()
	if err != nil {
		t.Fatalf("Failed to generate server certificate: %v", err)
	}

	t.Run("loads client TLS config", func(t *testing.T) {
		config, err := ClientTLSConfig()
		if err != nil {
			t.Fatalf("ClientTLSConfig failed: %v", err)
		}

		if config == nil {
			t.Fatal("Client TLS config is nil")
		}

		if len(config.Certificates) != 1 {
			t.Fatalf("Expected 1 certificate, got %d", len(config.Certificates))
		}

		if config.ServerName != "op-authd-local" {
			t.Errorf("Expected ServerName 'op-authd-local', got %q", config.ServerName)
		}

		if !config.InsecureSkipVerify {
			t.Error("Expected InsecureSkipVerify to be true for self-signed certificates")
		}
	})

	t.Run("fails when certificate doesn't exist", func(t *testing.T) {
		// Remove certificate files
		certPath := filepath.Join(tmpDir, "tls.crt")
		keyPath := filepath.Join(tmpDir, "tls.key")
		os.Remove(certPath)
		os.Remove(keyPath)

		_, err := ClientTLSConfig()
		if err == nil {
			t.Error("Expected error when certificate files don't exist")
		}
	})
}

func TestGenerateSelfSignedCert(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "test.crt")
	keyPath := filepath.Join(tmpDir, "test.key")

	err := generateSelfSignedCert(certPath, keyPath)
	if err != nil {
		t.Fatalf("generateSelfSignedCert failed: %v", err)
	}

	// Verify files were created
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("Certificate file not created")
	}

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("Key file not created")
	}

	// Try to load the generated certificate
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("Failed to load generated certificate: %v", err)
	}

	// Verify certificate has correct properties
	x509Cert := cert.Leaf
	if x509Cert == nil {
		// Parse the certificate since Leaf might be nil
		x509Cert, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			t.Fatalf("Failed to parse certificate: %v", err)
		}
	}

	if x509Cert.Subject.CommonName != "op-authd-local" {
		t.Errorf("Expected CommonName 'op-authd-local', got %q", x509Cert.Subject.CommonName)
	}

	// Verify certificate is valid for at least 300 days
	validDuration := x509Cert.NotAfter.Sub(x509Cert.NotBefore)
	if validDuration < 300*24*time.Hour {
		t.Errorf("Certificate validity duration too short: %v", validDuration)
	}
}

func TestLoadExistingCert(t *testing.T) {
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "test.crt")
	keyPath := filepath.Join(tmpDir, "test.key")

	// Generate a certificate first
	err := generateSelfSignedCert(certPath, keyPath)
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	t.Run("loads valid certificate", func(t *testing.T) {
		cert, err := loadExistingCert(certPath, keyPath)
		if err != nil {
			t.Fatalf("loadExistingCert failed: %v", err)
		}

		if cert.Leaf == nil {
			t.Error("Certificate Leaf is nil")
		}

		if cert.Leaf.Subject.CommonName != "op-authd-local" {
			t.Errorf("Expected CommonName 'op-authd-local', got %q", cert.Leaf.Subject.CommonName)
		}
	})

	t.Run("fails with non-existent files", func(t *testing.T) {
		_, err := loadExistingCert("nonexistent.crt", "nonexistent.key")
		if err == nil {
			t.Error("Expected error for non-existent certificate files")
		}
	})
}

func TestGetCertPaths(t *testing.T) {
	tmpDir := t.TempDir()
	originalGetStateDir := getStateDir
	getStateDir = func() (string, error) { return tmpDir, nil }
	defer func() { getStateDir = originalGetStateDir }()

	certPath, keyPath, err := getCertPaths()
	if err != nil {
		t.Fatalf("getCertPaths failed: %v", err)
	}

	expectedCert := filepath.Join(tmpDir, "tls.crt")
	expectedKey := filepath.Join(tmpDir, "tls.key")

	if certPath != expectedCert {
		t.Errorf("Expected cert path %q, got %q", expectedCert, certPath)
	}

	if keyPath != expectedKey {
		t.Errorf("Expected key path %q, got %q", expectedKey, keyPath)
	}
}
