package api

import (
	"testing"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/services"
)

func TestForecastCache(t *testing.T) {
	c := newForecastCache()
	user := uuid.New()
	res := &services.ForecastResult{}

	got, v := c.get(user)
	if got != nil {
		t.Fatal("empty cache returned a result")
	}
	c.set(user, v, res)
	if got, _ := c.get(user); got != res {
		t.Fatal("cached result not returned")
	}

	c.invalidate(user)
	if got, _ := c.get(user); got != nil {
		t.Fatal("result survived invalidate")
	}

	// A forecast that started before a write must not be stored.
	_, v = c.get(user)
	c.invalidate(user)
	c.set(user, v, res)
	if got, _ := c.get(user); got != nil {
		t.Fatal("stale result stored after a concurrent write")
	}

	// Other users are unaffected.
	other := uuid.New()
	_, v = c.get(other)
	c.set(other, v, res)
	c.invalidate(user)
	if got, _ := c.get(other); got != res {
		t.Fatal("invalidating one user cleared another")
	}
}
