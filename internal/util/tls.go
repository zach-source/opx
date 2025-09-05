package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// TLSConfig generates or loads TLS configuration for Unix socket encryption
func TLSConfig() (*tls.Config, error) {
	certPath, keyPath, err := getCertPaths()
	if err != nil {
		return nil, err
	}

	// Check if cert and key already exist and are valid
	if cert, err := loadExistingCert(certPath, keyPath); err == nil {
		if cert.Leaf != nil && cert.Leaf.NotAfter.After(time.Now().Add(24*time.Hour)) {
			// Certificate is valid and has >24 hours remaining
			return &tls.Config{
				Certificates: []tls.Certificate{cert},
				ServerName:   "op-authd-local", // For client verification
			}, nil
		}
	}

	// Generate new certificate if needed
	if err := generateSelfSignedCert(certPath, keyPath); err != nil {
		return nil, fmt.Errorf("failed to generate TLS certificate: %w", err)
	}

	cert, err := loadExistingCert(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load newly generated certificate: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   "op-authd-local",
	}, nil
}

// ClientTLSConfig returns TLS config for client connections
func ClientTLSConfig() (*tls.Config, error) {
	certPath, keyPath, err := getCertPaths()
	if err != nil {
		return nil, err
	}

	cert, err := loadExistingCert(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ServerName:         "op-authd-local",
		InsecureSkipVerify: true, // Self-signed cert, but we verify via token auth
	}, nil
}

func getCertPaths() (certPath, keyPath string, err error) {
	dir, err := getStateDir()
	if err != nil {
		return "", "", err
	}

	certPath = filepath.Join(dir, "tls.crt")
	keyPath = filepath.Join(dir, "tls.key")
	return certPath, keyPath, nil
}

// getStateDir allows for testing override
var getStateDir = StateDir

func loadExistingCert(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Parse the certificate to check expiration
	if cert.Leaf == nil {
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return tls.Certificate{}, err
		}
		cert.Leaf = x509Cert
	}

	return cert, nil
}

func generateSelfSignedCert(certPath, keyPath string) error {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"op-authd"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    "op-authd-local",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour), // Valid for 1 year
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:    []string{"localhost", "op-authd-local"},
	}

	// Generate the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return fmt.Errorf("failed to create certificate directory: %w", err)
	}

	// Write certificate file
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create certificate file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	}); err != nil {
		return fmt.Errorf("failed to write certificate: %w", err)
	}

	// Write private key file
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	privateKeyDER := x509.MarshalPKCS1PrivateKey(privateKey)

	if err := pem.Encode(keyFile, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyDER,
	}); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	return nil
}
