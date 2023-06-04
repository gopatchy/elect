package elect_test

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gopatchy/elect"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

type TestServer struct {
	Candidate *elect.Candidate

	listener *net.TCPListener
	srv      *http.Server
}

func NewTestServer(t *testing.T, signingKey string) *TestServer {
	ts := &TestServer{
		Candidate: elect.NewCandidate(1, signingKey),
		listener:  lo.Must(net.ListenTCP("tcp", nil)),
	}

	ts.srv = &http.Server{
		Handler:           ts.Candidate,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		err := ts.srv.Serve(ts.listener)
		require.ErrorIs(t, err, http.ErrServerClosed)
	}()

	return ts
}

func (ts *TestServer) Stop() {
	ts.srv.Close()
	ts.Candidate.Stop()
}

func (ts *TestServer) Addr() *net.TCPAddr {
	return ts.listener.Addr().(*net.TCPAddr)
}
