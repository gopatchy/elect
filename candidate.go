package elect

import (
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dchest/uniuri"
	"github.com/samber/lo"
)

// TODO: Ensure promotion takes longer than demotion

type Candidate struct {
	C <-chan CandidateState

	numVoters  int
	signingKey []byte
	stop       chan<- bool
	done       <-chan bool
	resp       voteResponse
	c          chan<- CandidateState

	votes map[string]*vote
	state CandidateState
	mu    sync.Mutex
}

type CandidateState string

var (
	StateLeader    CandidateState = "LEADER"
	StateNotLeader CandidateState = "NOT_LEADER"
)

func NewCandidate(numVoters int, signingKey string) *Candidate {
	stop := make(chan bool)
	done := make(chan bool)
	change := make(chan CandidateState, 100)

	c := &Candidate{
		C:          change,
		numVoters:  numVoters,
		signingKey: []byte(signingKey),
		votes:      map[string]*vote{},
		stop:       stop,
		done:       done,
		c:          change,
		resp: voteResponse{
			CandidateID: uniuri.New(),
		},
	}

	go c.loop(stop, done)

	return c
}

func (c *Candidate) Stop() {
	close(c.stop)
	<-c.done
}

func (c *Candidate) State() CandidateState {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.state
}

func (c *Candidate) IsLeader() bool {
	return c.State() == StateLeader
}

func (c *Candidate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(
			w,
			fmt.Sprintf("method %s not supported", r.Method),
			http.StatusMethodNotAllowed,
		)

		return
	}

	sig := r.Header.Get("Signature")
	if sig == "" {
		http.Error(
			w,
			"missing Signature header",
			http.StatusBadRequest,
		)

		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(
			w,
			fmt.Sprintf("Content-Type %s not supported", r.Header.Get("Content-Type")),
			http.StatusUnsupportedMediaType,
		)

		return
	}

	js, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(
			w,
			fmt.Sprintf("can't read request body: %s", err),
			http.StatusBadRequest,
		)

		return
	}

	if !hmac.Equal([]byte(sig), []byte(mac(js, c.signingKey))) {
		http.Error(
			w,
			"Signature verification failed",
			http.StatusBadRequest,
		)

		return
	}

	v := &vote{}

	err = json.Unmarshal(js, v)
	if err != nil {
		http.Error(
			w,
			fmt.Sprintf("can't parse request body: %s", err),
			http.StatusBadRequest,
		)

		return
	}

	if time.Since(v.VoteSent).Abs() > 15*time.Second {
		http.Error(
			w,
			fmt.Sprintf("excessive time difference (%.1f seconds); delay, replay, or clock skew", time.Since(v.VoteSent).Seconds()),
			http.StatusBadRequest,
		)
	}

	c.resp.ResponseSent = time.Now().UTC()

	js = lo.Must(json.Marshal(c.resp))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Signature", mac(js, c.signingKey))

	_, err = w.Write(js)
	if err != nil {
		http.Error(
			w,
			fmt.Sprintf("can't write response: %s", err),
			http.StatusInternalServerError,
		)

		return
	}

	c.vote(v)
}

func (c *Candidate) vote(v *vote) {
	v.received = time.Now()

	{
		c.mu.Lock()
		c.votes[v.VoterID] = v
		c.mu.Unlock()
	}

	c.elect()
}

func (c *Candidate) voteIfNo(v *vote) {
	if v.LastSeenCandidateID == c.resp.CandidateID {
		return
	}

	c.vote(v)
}

func (c *Candidate) elect() {
	no := 0
	yes := 0

	cutoff := time.Now().Add(-10 * time.Second)

	c.mu.Lock() /////////////

	for key, vote := range c.votes {
		if vote.received.Before(cutoff) {
			delete(c.votes, key)
			continue
		}

		if vote.LastSeenCandidateID != c.resp.CandidateID {
			no++
		}

		if vote.NumPollsSinceChange < 10 {
			continue
		}

		yes++
	}

	c.mu.Unlock() ////////////

	if no == 0 && yes > c.numVoters/2 {
		c.update(StateLeader)
	} else {
		c.update(StateNotLeader)
	}
}

func (c *Candidate) update(state CandidateState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == state {
		return
	}

	c.state = state
	c.c <- state
}

func (c *Candidate) loop(stop <-chan bool, done chan<- bool) {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	defer close(done)

	for {
		select {
		case <-stop:
			return

		case <-t.C:
			c.elect()
		}
	}
}
