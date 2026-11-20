package clash

import (
	"testing"
	stdtime "time"
)

func TestProfile(t *testing.T) {
	x := Profile{at: stdtime.Now(), cfg: p{n: 1}}
	x.rows[1].id = i{n: 2}
	x.rows[1].tags = make([]string, 1, 2)

	x.Reset()

	if !x.at.IsZero() || x.cfg.n != 0 || x.rows[1].id.n != 0 || len(x.rows[1].tags) != 0 || cap(x.rows[1].tags) != 2 {
		t.Errorf("profile was not reset: %+v", x)
	}

	if time != stdtime.Second {
		t.Errorf("package variable time changed")
	}
}
