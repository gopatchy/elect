package elect

import (
	"crypto/hmac"
	"encoding/json"
	"log"
	"time"

	"github.com/dchest/uniuri"
	"github.com/go-resty/resty/v2"
	"github.com/samber/lo"
)

type Voter struct {
	client     *resty.Client
	signingKey []byte
	update     chan<- time.Duration
	done       <-chan bool
	vote       vote
	candidates []*Candidate
}

type vote struct {
	VoterID             string `json:"voterID"`
	LastSeenCandidateID string `json:"lastSeenCandidateID"`
	NumPollsSinceChange int    `json:"numPollsSinceChange"`
	// TODO: Add timestamp

	// Used internally by Candidate
	received time.Time
}

type voteResponse struct {
	CandidateID string `json:"candidateID"`
	// TODO: Add timestamp
}

func NewVoter(url string, signingKey string) *Voter {
	update := make(chan time.Duration)
	done := make(chan bool)

	v := &Voter{
		client: resty.New().
			SetCloseConnection(true).
			SetBaseURL(url),
		signingKey: []byte(signingKey),
		update:     update,
		done:       done,
		vote: vote{
			VoterID: uniuri.New(),
		},
	}

	go v.loop(update, done)

	return v
}

func (v *Voter) Stop() {
	close(v.update)
	<-v.done
}

func (v *Voter) AddCandidate(c *Candidate) {
	v.candidates = append(v.candidates, c)
}

func (v *Voter) loop(update <-chan time.Duration, done chan<- bool) {
	// TODO: Need a JitterTicker
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	defer close(done)

	for {
		if !v.poll(update, t) {
			break
		}
	}
}

func (v *Voter) poll(update <-chan time.Duration, t *time.Ticker) bool {
	t2 := &time.Timer{}

	if v.vote.NumPollsSinceChange <= 10 {
		t2 = time.NewTimer(100 * time.Millisecond)
		defer t2.Stop()
	}

	select {
	case <-t2.C:
		v.sendVote()

	case <-t.C:
		v.sendVote()

	case period, ok := <-update:
		if !ok {
			return false
		}

		t.Reset(period)
	}

	return true
}

func (v *Voter) sendVote() {
	for _, c := range v.candidates {
		c.voteIfNo(&v.vote)
	}

	js := lo.Must(json.Marshal(v.vote))

	resp, err := v.client.R().
		SetHeader("Signature", mac(js, v.signingKey)).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetBody(js).
		Post("")
	if err != nil {
		log.Printf("vote response: %s", err)

		v.vote.NumPollsSinceChange = 0

		return
	}

	if resp.IsError() {
		v.log("response: [%d] %s\n%s", resp.StatusCode(), resp.Status(), resp.String())

		v.vote.NumPollsSinceChange = 0

		return
	}

	sig := resp.Header().Get("Signature")
	if sig == "" {
		v.log("missing Signature response header")
		return
	}

	if !hmac.Equal([]byte(sig), []byte(mac(resp.Body(), v.signingKey))) {
		v.log("invalid Signature response header")
		return
	}

	vr := &voteResponse{}

	err = json.Unmarshal(resp.Body(), vr)
	if err != nil {
		v.log("invalid response: %s", resp.String())
		return
	}

	if vr.CandidateID == v.vote.LastSeenCandidateID {
		v.vote.NumPollsSinceChange++
	} else {
		v.vote.LastSeenCandidateID = vr.CandidateID
		v.vote.NumPollsSinceChange = 0
	}
}

func (v *Voter) log(format string, args ...any) {
	log.Printf("[voter] "+format, args...)
}
