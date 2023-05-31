package elect

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
}

type vote struct {
	VoterID             string `json:"voterID"`
	LastSeenCandidateID string `json:"lastSeenCandidateID"`
	NumPollsSinceChange int    `json:"numPollsSinceChange"`
}

type voteResponse struct {
	CandidateID string `json:"candidateID"`
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

func (v *Voter) loop(update <-chan time.Duration, done chan<- bool) {
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

	if v.vote.NumPollsSinceChange < 10 {
		t2 = time.NewTimer(100 * time.Millisecond)
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
	js := lo.Must(json.Marshal(v.vote))

	genMAC := hmac.New(sha256.New, v.signingKey)
	genMAC.Write(js)
	mac := fmt.Sprintf("%x", genMAC.Sum(nil))

	vr := &voteResponse{}

	resp, err := v.client.R().
		SetHeader("Signature", mac).
		SetBody(js).
		SetResult(vr).
		Post("_vote")
	if err != nil {
		log.Printf("_vote response: %s", err)
		return
	}

	if resp.IsError() {
		log.Printf("_vote response: [%d] %s\n%s", resp.StatusCode(), resp.Status(), resp.String())
		return
	}

	if vr.CandidateID == v.vote.LastSeenCandidateID {
		v.vote.NumPollsSinceChange++
	} else {
		v.vote.LastSeenCandidateID = vr.CandidateID
		v.vote.NumPollsSinceChange = 0
	}
}
