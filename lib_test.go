package elect_test

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/dchest/uniuri"
	"github.com/gopatchy/elect"
	"github.com/gopatchy/proxy"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

type TestServer struct {
	Candidate *elect.Candidate

	listener *net.TCPListener
	srv      *http.Server
}

type TestSystem struct {
	signingKey string
	servers    []*TestServer
	voters     []*elect.Voter
	proxy      *proxy.Proxy
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

func NewTestSystem(t *testing.T, num int) *TestSystem {
	ts := &TestSystem{
		signingKey: uniuri.New(),
	}

	for i := 0; i < num; i++ {
		ts.servers = append(ts.servers, NewTestServer(t, ts.signingKey))
	}

	ts.proxy = lo.Must(proxy.NewProxy(t, ts.Server(0).Addr()))

	url := fmt.Sprintf("http://%s/", ts.proxy.Addr())

	for i := 0; i < num; i++ {
		ts.voters = append(ts.voters, elect.NewVoter(url, ts.signingKey, ts.Candidate(i)))
	}

	return ts
}

func (ts *TestSystem) Stop() {
	for _, s := range ts.servers {
		s.Stop()
	}

	for _, v := range ts.voters {
		v.Stop()
	}

	ts.proxy.Close()
}

func (ts *TestSystem) SetServer(i int) {
	ts.proxy.SetBackend(ts.Server(i).Addr())
}

func (ts *TestSystem) Server(i int) *TestServer {
	return ts.servers[i]
}

func (ts *TestSystem) Candidate(i int) *elect.Candidate {
	return ts.servers[i].Candidate
}

func (ts *TestSystem) Voter(i int) *elect.Voter {
	return ts.voters[i]
}
