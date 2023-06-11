package elect

import (
	"math/rand"
	"time"
)

func RandDurationN(n time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(n))) //nolint:gosec
}
