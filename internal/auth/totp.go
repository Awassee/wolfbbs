package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

func GenerateTOTPSecret() (string, error) {
	bytes := make([]byte, 10)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	return enc.EncodeToString(bytes), nil
}

func VerifyTOTP(secret string, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	window := int64(1)
	counter := now.Unix() / 30
	for offset := -window; offset <= window; offset++ {
		if expected := hotp(secret, counter+offset); expected == code {
			return true
		}
	}
	return false
}

func hotp(secret string, counter int64) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf)
	d := mac.Sum(nil)
	offset := int(d[len(d)-1] & 0x0f)
	binaryCode := (int(d[offset]&0x7f) << 24) | (int(d[offset+1]&0xff) << 16) | (int(d[offset+2]&0xff) << 8) | int(d[offset+3]&0xff)
	otp := binaryCode % int(math.Pow10(6))
	return fmt.Sprintf("%06d", otp)
}
