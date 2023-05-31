package elect

import (
	"time"

	"github.com/dchest/uniuri"
	"github.com/go-resty/resty/v2"
)

type Voter struct {
	client     *resty.Client
	instanceID string
	signingKey string
	update     chan<- time.Duration
}

func NewVoter(url string, signingKey string) *Voter {
	update := make(chan time.Duration) // intentionally 0-capacity

	v := &Voter{
		client: resty.New().
			SetBaseURL(url),
		instanceID: uniuri.New(),
		signingKey: signingKey,
		update:     update,
	}

	go v.loop(update)

	return v
}

func (v *Voter) Stop() {
	close(v.update)
}

func (v *Voter) loop(update <-chan time.Duration) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			v.vote()

		case period, ok := <-update:
			if !ok {
				return
			}

			t.Reset(period)
		}
	}
}

func (v *Voter) vote() {
}
