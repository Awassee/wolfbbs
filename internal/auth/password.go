package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

const (
	HashBcrypt       = "bcrypt"
	HashPBKDF2SHA256 = "pbkdf2-sha256"

	defaultPBKDF2Iterations = 210000
	defaultPBKDF2SaltBytes  = 16
	defaultPBKDF2KeyBytes   = 32
)

type HashPolicy struct {
	Algorithm        string
	PBKDF2Iterations int
	PBKDF2SaltBytes  int
	UpgradeOnLogin   bool
}

func defaultHashPolicy() HashPolicy {
	return HashPolicy{
		Algorithm:        HashBcrypt,
		PBKDF2Iterations: defaultPBKDF2Iterations,
		PBKDF2SaltBytes:  defaultPBKDF2SaltBytes,
		UpgradeOnLogin:   true,
	}
}

func normalizeHashAlgorithm(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pbkdf2", "pbkdf2-sha256", "pbkdf2_sha256":
		return HashPBKDF2SHA256
	case "", "bcrypt":
		return HashBcrypt
	default:
		return HashBcrypt
	}
}

func (p HashPolicy) normalize() HashPolicy {
	n := p
	n.Algorithm = normalizeHashAlgorithm(n.Algorithm)
	if n.PBKDF2Iterations <= 0 {
		n.PBKDF2Iterations = defaultPBKDF2Iterations
	}
	if n.PBKDF2SaltBytes <= 0 {
		n.PBKDF2SaltBytes = defaultPBKDF2SaltBytes
	}
	return n
}

func hashPassword(password string, policy HashPolicy) (string, error) {
	policy = policy.normalize()
	switch policy.Algorithm {
	case HashPBKDF2SHA256:
		salt := make([]byte, policy.PBKDF2SaltBytes)
		if _, err := rand.Read(salt); err != nil {
			return "", err
		}
		dk := pbkdf2.Key([]byte(password), salt, policy.PBKDF2Iterations, defaultPBKDF2KeyBytes, sha256.New)
		return HashPBKDF2SHA256 + "$" + strconv.Itoa(policy.PBKDF2Iterations) + "$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(dk), nil
	default:
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return "", err
		}
		return string(hash), nil
	}
}

func verifyPassword(storedHash string, password string) (bool, error) {
	storedHash = strings.TrimSpace(storedHash)
	if storedHash == "" {
		return false, nil
	}
	if strings.HasPrefix(storedHash, HashPBKDF2SHA256+"$") {
		return verifyPBKDF2Hash(storedHash, password)
	}
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, nil
	}
	return false, err
}

func verifyPBKDF2Hash(storedHash, password string) (bool, error) {
	parts := strings.Split(storedHash, "$")
	if len(parts) != 4 {
		return false, nil
	}
	if parts[0] != HashPBKDF2SHA256 {
		return false, nil
	}
	iters, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || iters <= 0 {
		return false, nil
	}
	salt, err := hex.DecodeString(strings.TrimSpace(parts[2]))
	if err != nil || len(salt) == 0 {
		return false, nil
	}
	want, err := hex.DecodeString(strings.TrimSpace(parts[3]))
	if err != nil || len(want) == 0 {
		return false, nil
	}
	got := pbkdf2.Key([]byte(password), salt, iters, len(want), sha256.New)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func shouldUpgradeHash(storedHash string, policy HashPolicy) bool {
	policy = policy.normalize()
	if !policy.UpgradeOnLogin {
		return false
	}
	switch policy.Algorithm {
	case HashPBKDF2SHA256:
		return !strings.HasPrefix(strings.TrimSpace(storedHash), HashPBKDF2SHA256+"$")
	default:
		return false
	}
}
