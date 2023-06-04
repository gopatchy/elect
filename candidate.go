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

type Candidate struct {
	C <-chan CandidateState

	numVoters  int
	signingKey []byte
	stop       chan bool
	done       chan bool
	resp       voteResponse
	c          chan<- CandidateState

	votes    map[string]*vote
	state    CandidateState
	firstYes time.Time
	mu       sync.Mutex
}

type CandidateState string

var (
	StateLeader    CandidateState = "LEADER"
	StateNotLeader CandidateState = "NOT_LEADER"
)

const (
	maxVotePeriod  = 5 * time.Second
	voteTimeout    = 10 * time.Second
	leadershipWait = 15 * time.Second

	maxFastVotePeriod = 100 * time.Millisecond
)

func NewCandidate(numVoters int, signingKey string) *Candidate {
	change := make(chan CandidateState, 100)

	c := &Candidate{
		C:          change,
		numVoters:  numVoters,
		signingKey: []byte(signingKey),
		votes:      map[string]*vote{},
		stop:       make(chan bool),
		done:       make(chan bool),
		c:          change,
		resp: voteResponse{
			CandidateID: uniuri.New(),
		},
	}

	go c.loop()

	return c
}

func (c *Candidate) Stop() {
	// Lock not required

	close(c.stop)
	<-c.done
}

func (c *Candidate) State() CandidateState {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.state
}

func (c *Candidate) IsLeader() bool {
	// Lock not required

	return c.State() == StateLeader
}

func (c *Candidate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

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

func (c *Candidate) VoteIfNo(v vote) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v.LastSeenCandidateID == c.resp.CandidateID {
		return
	}

	c.vote(&v)
}

func (c *Candidate) vote(v *vote) {
	// Must hold lock to call

	v.received = time.Now()
	c.votes[v.VoterID] = v

	c.elect()
}

func (c *Candidate) elect() {
	// Must hold lock to call

	no := 0
	yes := 0

	for key, vote := range c.votes {
		if time.Since(vote.received) > voteTimeout {
			// Remove stale vote from consideration
			delete(c.votes, key)
			continue
		}

		if vote.LastSeenCandidateID != c.resp.CandidateID {
			// Hard no; voted for someone else
			no++
		}

		if vote.NumPollsSinceChange < 10 {
			// Soft no; voted for us but not enough times in a row
			continue
		}

		yes++
	}

	if no > 0 || yes <= c.numVoters/2 {
		// We lost the vote
		c.firstYes = time.Time{}
		c.update(StateNotLeader)

		return
	}

	if c.firstYes.IsZero() {
		// First round of "yes" voting since the last "no" vote
		c.firstYes = time.Now()
	}

	if time.Since(c.firstYes) < leadershipWait {
		// Not enough time in "yes" state
		c.update(StateNotLeader)
		return
	}

	// All checks passed
	c.update(StateLeader)
}

func (c *Candidate) update(state CandidateState) {
	// Must hold lock to call

	if c.state == state {
		return
	}

	c.state = state
	c.c <- state
}

func (c *Candidate) loop() {
	// Lock not required

	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	defer close(c.done)

	for {
		select {
		case <-c.stop:
			return

		case <-t.C:
			c.lockElect()
		}
	}
}

func (c *Candidate) lockElect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.elect()
}
