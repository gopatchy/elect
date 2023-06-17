package elect

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"log"
	"time"

	"github.com/dchest/uniuri"
	"github.com/go-resty/resty/v2"
	"github.com/gopatchy/event"
	"github.com/samber/lo"
)

type Voter struct {
	// used by user and loop() goroutines
	update chan time.Duration
	done   chan bool

	// used by loop() goroutine only
	client     *resty.Client
	signingKey []byte
	vote       vote
	period     time.Duration
}

type vote struct {
	VoterID             string    `json:"voterID"`
	LastSeenCandidateID string    `json:"lastSeenCandidateID"`
	NumPollsSinceChange int       `json:"numPollsSinceChange"`
	VoteSent            time.Time `json:"voteSent"`

	// Used internally by Candidate
	received time.Time
}

type voteResponse struct {
	CandidateID  string    `json:"candidateID"`
	ResponseSent time.Time `json:"responseSent"`
}

func NewVoter(ctx context.Context, ec *event.Client, url string, signingKey string) *Voter {
	v := &Voter{
		client: resty.New().
			SetCloseConnection(true).
			SetBaseURL(url),
		signingKey: []byte(signingKey),
		update:     make(chan time.Duration),
		done:       make(chan bool),
		vote: vote{
			VoterID: uniuri.New(),
		},
		period: maxVotePeriod,
	}

	go v.loop(ctx, ec)

	return v
}

func (v *Voter) Stop() {
	if v.update == nil {
		return
	}

	close(v.update)
	<-v.done
	v.update = nil
}

func (v *Voter) loop(ctx context.Context, ec *event.Client) {
	defer close(v.done)

	for {
		if !v.poll(ctx, ec) {
			break
		}
	}
}

func (v *Voter) poll(ctx context.Context, ec *event.Client) bool {
	t := time.NewTimer(RandDurationN(v.period))
	defer t.Stop()

	t2 := &time.Timer{}

	if v.vote.NumPollsSinceChange <= 10 {
		t2 = time.NewTimer(RandDurationN(maxFastVotePeriod))
		defer t2.Stop()
	}

	select {
	case <-t.C:
		v.sendVote(ctx, ec)

	case <-t2.C:
		v.sendVote(ctx, ec)

	case period, ok := <-v.update:
		if !ok {
			return false
		}

		v.period = period
	}

	return true
}

func (v *Voter) sendVote(ctx context.Context, ec *event.Client) {
	v.vote.VoteSent = time.Now().UTC()

	js := lo.Must(json.Marshal(v.vote))

	resp, err := v.client.R().
		SetHeader("Signature", mac(js, v.signingKey)).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetBody(js).
		Post("")
	if err != nil {
		log.Printf("error: %s", err)

		v.vote.NumPollsSinceChange = 0

		return
	}

	if resp.IsError() {
		v.log(ctx, ec,
			"event", "error",
			"response", resp.StatusCode(),
		)

		v.vote.NumPollsSinceChange = 0

		return
	}

	sig := resp.Header().Get("Signature")
	if sig == "" {
		v.log(ctx, ec,
			"event", "error",
			"error", "missing Signature response header",
		)

		return
	}

	if !hmac.Equal([]byte(sig), []byte(mac(resp.Body(), v.signingKey))) {
		v.log(ctx, ec,
			"event", "error",
			"error", "invalid Signature response header",
		)

		return
	}

	vr := &voteResponse{}

	err = json.Unmarshal(resp.Body(), vr)
	if err != nil {
		v.log(ctx, ec,
			"event", "error",
			"error", err,
		)

		return
	}

	if time.Since(vr.ResponseSent).Abs() > 15*time.Second {
		v.log(ctx, ec,
			"event", "error",
			"error", "excessive time difference",
		)
	}

	if vr.CandidateID == v.vote.LastSeenCandidateID {
		v.vote.NumPollsSinceChange++
	} else {
		v.vote.LastSeenCandidateID = vr.CandidateID
		v.vote.NumPollsSinceChange = 0
	}
}

func (v *Voter) log(ctx context.Context, ec *event.Client, vals ...any) {
	ec.Log(ctx, append([]any{
		"library", "elect",
		"subsystem", "voter",
	}, vals...)...)
}
