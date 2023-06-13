package elect_test

import (
	"testing"
	"time"

	"github.com/gopatchy/elect"
	"github.com/stretchr/testify/require"
)

func TestOne(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 1, 1)
	defer ts.Stop()

	require.False(t, ts.Candidate(0).IsLeader())

	require.Eventually(t, ts.Candidate(0).IsLeader, 20*time.Second, 100*time.Millisecond)
}

func TestThree(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 3, 3)
	defer ts.Stop()

	require.False(t, ts.Candidate(0).IsLeader())
	require.False(t, ts.Candidate(1).IsLeader())
	require.False(t, ts.Candidate(2).IsLeader())

	{
		w := NewWaiter()

		w.Async(func() { require.Eventually(t, ts.Candidate(0).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(1).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(2).IsLeader, 20*time.Second, 100*time.Millisecond) })

		w.Wait()
	}

	require.Equal(t, <-ts.Candidate(0).C, elect.StateLeader)
	require.Empty(t, ts.Candidate(1).C)
	require.Empty(t, ts.Candidate(2).C)
}

func TestFailover(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 3, 3)
	defer ts.Stop()

	require.False(t, ts.Candidate(0).IsLeader())
	require.False(t, ts.Candidate(1).IsLeader())
	require.False(t, ts.Candidate(2).IsLeader())

	{
		w := NewWaiter()

		w.Async(func() { require.Eventually(t, ts.Candidate(0).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(1).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(2).IsLeader, 20*time.Second, 100*time.Millisecond) })

		w.Wait()
	}

	require.Equal(t, <-ts.Candidate(0).C, elect.StateLeader)
	require.Empty(t, ts.Candidate(1).C)
	require.Empty(t, ts.Candidate(2).C)

	ts.SetServer(1)

	require.Eventually(t, func() bool { return !ts.Candidate(0).IsLeader() }, 15*time.Second, 100*time.Millisecond)
	// New candidate must not get leadership before old candidate loses it
	require.False(t, ts.Candidate(1).IsLeader())
	require.False(t, ts.Candidate(2).IsLeader())

	require.Equal(t, <-ts.Candidate(0).C, elect.StateNotLeader)
	require.Empty(t, ts.Candidate(1).C)
	require.Empty(t, ts.Candidate(2).C)

	{
		w := NewWaiter()

		w.Async(func() { require.Eventually(t, ts.Candidate(1).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(0).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(2).IsLeader, 20*time.Second, 100*time.Millisecond) })

		w.Wait()
	}

	require.Equal(t, <-ts.Candidate(1).C, elect.StateLeader)
	require.Empty(t, ts.Candidate(0).C)
	require.Empty(t, ts.Candidate(2).C)
}

func TestPartialVotes(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 3, 3)
	defer ts.Stop()

	ts.Proxy(0).SetRefuse(true)

	require.False(t, ts.Candidate(0).IsLeader())
	require.False(t, ts.Candidate(1).IsLeader())
	require.False(t, ts.Candidate(2).IsLeader())

	{
		w := NewWaiter()

		w.Async(func() { require.Eventually(t, ts.Candidate(0).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(1).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(2).IsLeader, 20*time.Second, 100*time.Millisecond) })

		w.Wait()
	}

	require.Equal(t, <-ts.Candidate(0).C, elect.StateLeader)
	require.Empty(t, ts.Candidate(1).C)
	require.Empty(t, ts.Candidate(2).C)
}

func TestSplitVotes(t *testing.T) {
	t.Parallel()

	ts := NewTestSystem(t, 3, 3)
	defer ts.Stop()

	ts.SetServerForVoter(1, 1)
	ts.SetServerForVoter(1, 2)

	require.False(t, ts.Candidate(0).IsLeader())
	require.False(t, ts.Candidate(1).IsLeader())
	require.False(t, ts.Candidate(2).IsLeader())

	{
		w := NewWaiter()

		w.Async(func() { require.Eventually(t, ts.Candidate(1).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(0).IsLeader, 20*time.Second, 100*time.Millisecond) })
		w.Async(func() { require.Never(t, ts.Candidate(2).IsLeader, 20*time.Second, 100*time.Millisecond) })

		w.Wait()
	}

	require.Equal(t, <-ts.Candidate(1).C, elect.StateLeader)
	require.Empty(t, ts.Candidate(0).C)
	require.Empty(t, ts.Candidate(2).C)
}
