package elect

import (
	"github.com/dchest/uniuri"
	"github.com/go-resty/resty/v2"
)

type Elect struct {
	client     *resty.Client
	instanceID string
	signingKey string
}

func New(url string, signingKey string) *Elect {
	e := &Elect{
		client: resty.New().
			SetBaseURL(url),
		instanceID: uniuri.New(),
		signingKey: signingKey,
	}

	return e
}
