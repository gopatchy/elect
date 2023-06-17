package elect_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/dchest/uniuri"
	"github.com/gopatchy/elect"
	"github.com/gopatchy/event"
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
	proxies    []*proxy.Proxy
}

type Waiter struct {
	chans []<-chan bool
}

func NewTestServer(t *testing.T, numVoters int, signingKey string) *TestServer {
	ts := &TestServer{
		Candidate: elect.NewCandidate(numVoters, signingKey),
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

func NewTestSystem(t *testing.T, numCandidates, numVoters int) *TestSystem {
	ctx := context.Background()

	ec := event.New()
	defer ec.Close()

	ts := &TestSystem{
		signingKey: uniuri.New(),
	}

	for i := 0; i < numCandidates; i++ {
		ts.servers = append(ts.servers, NewTestServer(t, numVoters, ts.signingKey))
	}

	for i := 0; i < numVoters; i++ {
		ts.proxies = append(ts.proxies, proxy.NewProxy(t, ts.Server(0).Addr()))
		ts.voters = append(ts.voters, elect.NewVoter(ctx, ec, ts.Proxy(i).HTTP(), ts.signingKey))
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

	for _, p := range ts.proxies {
		p.Close()
	}
}

func (ts *TestSystem) SetServer(server int) {
	for _, p := range ts.proxies {
		p.SetBackend(ts.Server(server).Addr())
	}
}

func (ts *TestSystem) SetServerForVoter(server, voter int) {
	ts.Proxy(voter).SetBackend(ts.Server(server).Addr())
}

func (ts *TestSystem) Candidate(i int) *elect.Candidate {
	return ts.servers[i].Candidate
}

func (ts *TestSystem) Proxy(i int) *proxy.Proxy {
	return ts.proxies[i]
}

func (ts *TestSystem) Server(i int) *TestServer {
	return ts.servers[i]
}

func (ts *TestSystem) Voter(i int) *elect.Voter {
	return ts.voters[i]
}

func NewWaiter() *Waiter {
	return &Waiter{}
}

func (w *Waiter) Wait() {
	for _, ch := range w.chans {
		<-ch
	}
}

func (w *Waiter) Async(cb func()) {
	ch := make(chan bool)
	w.chans = append(w.chans, ch)

	go func() {
		defer close(ch)
		cb()
	}()
}
