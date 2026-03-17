package content

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wolfbbs/internal/domain"
	"wolfbbs/internal/repository"
)

func TestNNTPTLSStartAndQuit(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t)

	boards := repository.NewInMemoryBoardRepository()
	messages := repository.NewInMemoryMessageRepository()
	board := &domain.Board{Name: "TLS Board"}
	if err := boards.Create(board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if err := messages.CreateMessage(&domain.Message{
		BoardID:   board.ID,
		AuthorID:  2,
		Subject:   "TLS Subject",
		Body:      "TLS body",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create message: %v", err)
	}

	srv := NewNNTPTLSServer("127.0.0.1:0", certPath, keyPath, boards, messages)
	if err := srv.Start(); err != nil {
		t.Fatalf("start nntps server: %v", err)
	}
	defer srv.Close()

	conn, err := tls.Dial("tcp", srv.Addr(), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("dial nntps: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read nntps greeting: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(line), "200 ") {
		t.Fatalf("expected 200 greeting, got %q", line)
	}
	if _, err := conn.Write([]byte("QUIT\r\n")); err != nil {
		t.Fatalf("write quit: %v", err)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read quit response: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(line), "205 ") {
		t.Fatalf("expected 205 quit response, got %q", line)
	}
}

func writeSelfSignedCert(t *testing.T) (string, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("write cert pem: %v", err)
	}
	_ = certOut.Close()
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	keyDER := x509.MarshalPKCS1PrivateKey(priv)
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("write key pem: %v", err)
	}
	_ = keyOut.Close()
	return certPath, keyPath
}
