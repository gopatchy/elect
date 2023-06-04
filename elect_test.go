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

func TestThree(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 3)
	defer ts.Stop()

	require.Eventually(t, ts.Candidate(0).IsLeader, 15*time.Second, 100*time.Millisecond)
}

func TestFailover(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 3)
	defer ts.Stop()

	require.Eventually(t, ts.Candidate(0).IsLeader, 15*time.Second, 100*time.Millisecond)

	ts.SetServer(1)

	require.Eventually(t, func() bool { return !ts.Candidate(0).IsLeader() }, 15*time.Second, 100*time.Millisecond)

	// TODO: Check that new candidate doesn't become leader before old candidate loses it
	// require.False(t, ts.Candidate(1).IsLeader())

	require.Eventually(t, ts.Candidate(1).IsLeader, 15*time.Second, 100*time.Millisecond)
}
