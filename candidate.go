package elect

import (
	"crypto/hmac"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
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

	votes      map[string]*vote
	state      CandidateState
	forceState CandidateState
	firstYes   time.Time
	mu         sync.RWMutex
}

type CandidateState int

const (
	StateUndefined CandidateState = iota
	StateLeader
	StateNotLeader

	maxVotePeriod  = 5 * time.Second
	voteTimeout    = 10 * time.Second
	leadershipWait = 15 * time.Second

	maxFastVotePeriod = 100 * time.Millisecond
)

var StateName = map[CandidateState]string{
	StateLeader:    "LEADER",
	StateNotLeader: "NOT_LEADER",
}

var electForceState = flag.String("elect-force-state", "", "'', 'leader', 'notleader'")

func NewCandidate(numVoters int, signingKey string) *Candidate {
	change := make(chan CandidateState, 100)

	c := &Candidate{
		C:          change,
		numVoters:  numVoters,
		signingKey: []byte(signingKey),
		votes:      map[string]*vote{},
		state:      StateNotLeader,
		forceState: getForceState(),
		stop:       make(chan bool),
		done:       make(chan bool),
		c:          change,
		resp: voteResponse{
			CandidateID: uniuri.New(),
		},
	}

	if c.forceState != StateUndefined {
		c.log("state forced to %s", StateName[c.forceState])
		c.state = c.forceState
	}

	go c.loop()

	return c
}

func (c *Candidate) Stop() {
	close(c.stop)
	<-c.done
}

func (c *Candidate) State() CandidateState {
	c.mu.RLock()
	defer c.mu.RUnlock()

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

	resp := c.resp
	resp.ResponseSent = time.Now().UTC()
	js = lo.Must(json.Marshal(resp))

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

	c.elect(v)
}

func (c *Candidate) elect(v *vote) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := StateNotLeader
	no := 0
	yes := 0

	defer func() {
		if c.state == state {
			return
		}

		c.log(
			"transitioning %s -> %s (no=%d yes=%d max_no=0 min_yes=%d)",
			StateName[c.state],
			StateName[state],
			no,
			yes,
			c.numVoters/2+1,
		)

		c.state = state

		select {
		case c.c <- state:
		default:
		}
	}()

	if c.forceState != StateUndefined {
		c.state = c.forceState
		return
	}

	if v != nil {
		v.received = time.Now()
		c.votes[v.VoterID] = v
	}

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

	if no > 0 || yes < c.numVoters/2+1 {
		// We lost the vote
		c.firstYes = time.Time{}
		state = StateNotLeader

		return
	}

	if c.firstYes.IsZero() {
		// First round of "yes" voting since the last "no" vote
		c.firstYes = time.Now()
	}

	if time.Since(c.firstYes) < leadershipWait {
		// Not enough time in "yes" state
		state = StateNotLeader
		return
	}

	// All checks passed
	state = StateLeader
}

func (c *Candidate) loop() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	defer close(c.done)

	for {
		select {
		case <-c.stop:
			return

		case <-t.C:
			c.elect(nil)
		}
	}
}

func (c *Candidate) log(format string, args ...any) {
	log.Printf("[candidate] "+format, args...)
}

func getForceState() CandidateState {
	switch *electForceState {
	case "":
		return StateUndefined

	case "leader":
		return StateLeader

	case "not-leader":
		fallthrough
	case "not_leader":
		fallthrough
	case "notleader":
		return StateNotLeader

	default:
		panic("invalid --elect-force-state")
	}
}
