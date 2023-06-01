package elect

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

func mac(payload, signingKey []byte) string {
	gen := hmac.New(sha256.New, signingKey)
	gen.Write(payload)

	return fmt.Sprintf("%x", gen.Sum(nil))
}
