package elect_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/dchest/uniuri"
	"github.com/gopatchy/elect"
	"github.com/gopatchy/proxy"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestSimple(t *testing.T) {
	t.Parallel()

	signingKey := uniuri.New()

	ts := NewTestServer(t, signingKey)
	defer ts.Stop()

	p := lo.Must(proxy.NewProxy(t, ts.Addr()))
	defer p.Close()

	url := fmt.Sprintf("http://%s/", p.Addr())

	v := elect.NewVoter(url, signingKey)
	defer v.Stop()

	require.Eventually(t, ts.Candidate.IsLeader, 15*time.Second, 100*time.Millisecond)
}
