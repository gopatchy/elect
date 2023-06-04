package elect_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOne(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 1)
	defer ts.Stop()

	require.Eventually(t, ts.Candidate(0).IsLeader, 15*time.Second, 100*time.Millisecond)
}
