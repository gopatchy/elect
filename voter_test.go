package elect_test

import (
	"testing"
	"time"

	"github.com/gopatchy/elect"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	v := elect.NewVoter("https://[::1]:1234", "abc123")
	require.NotNil(t, v)

	time.Sleep(1 * time.Second)

	defer v.Stop()
}
