package elect_test

import (
	"testing"

	"github.com/gopatchy/elect"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	c := elect.New("[::1]:1234", "abc123")
	require.NotNil(t, c)
}
