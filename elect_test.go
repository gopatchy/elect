package elect_test

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gopatchy/elect"
	"github.com/stretchr/testify/require"
)

func TestSimple(t *testing.T) {
	t.Parallel()

	c := elect.NewCandidate(1, "abc123")

	defer c.Stop()

	listener, err := net.ListenTCP("tcp", nil)
	require.NoError(t, err)

	srv := &http.Server{
		Handler:           c,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		err := srv.Serve(listener)
		require.ErrorIs(t, err, http.ErrServerClosed)
	}()

	defer srv.Close()

	v := elect.NewVoter(fmt.Sprintf("http://%s/", listener.Addr()), "abc123")
	require.NotNil(t, v)

	defer v.Stop()

	require.Eventually(t, c.IsLeader, 15*time.Second, 100*time.Millisecond)
}
