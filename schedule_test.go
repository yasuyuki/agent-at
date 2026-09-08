package main

import (
	"context"
	"testing"
	"time"
)

func TestParseAt(t *testing.T) {
	utc := time.Date(2026, 12, 31, 23, 59, 30, 0, time.UTC)
	for _, tc := range []struct{ in, want string }{
		{"23:59:31", "2026-12-31T23:59:31Z"},
		{"23:59:30", "2027-01-01T23:59:30Z"},
		{"00:00", "2027-01-01T00:00:00Z"},
		{"2028-02-29T12:00", "2028-02-29T12:00:00Z"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseAt(tc.in, utc)
			if err != nil || got.Format(time.RFC3339) != tc.want {
				t.Fatalf("got %v, %v; want %s", got, err, tc.want)
			}
		})
	}
	for _, v := range []string{"", "1:00", "24:00", "12:60", "12:00:60", "2027-02-29T12:00", "2026-12-31T23:59:30", "2026-01-01T00:00", "2027-01-01T00:00Z", "2027-01-01 00:00", "12:00:00.1"} {
		if _, err := parseAt(v, utc); err == nil {
			t.Errorf("accepted %q", v)
		}
	}
}
func TestDST(t *testing.T) {
	for _, tc := range []struct {
		zone, now, input string
		valid            bool
	}{
		{"America/New_York", "2026-01-01T00:00:00Z", "2026-03-08T02:30", false},
		{"America/New_York", "2026-01-01T00:00:00Z", "2026-11-01T01:30", false},
		{"America/New_York", "2026-01-01T00:00:00Z", "2026-03-08T03:30", true},
		{"Australia/Lord_Howe", "2026-01-01T00:00:00Z", "2026-04-05T01:45", false},
		{"Australia/Lord_Howe", "2026-01-01T00:00:00Z", "2026-10-04T02:15", false},
		{"Pacific/Apia", "2011-01-01T00:00:00Z", "2011-12-30T12:00", false},
		{"America/New_York", "2026-03-08T05:00:00Z", "02:30", false},
	} {
		t.Run(tc.zone+tc.input, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			now, _ := time.Parse(time.RFC3339, tc.now)
			_, err = parseAt(tc.input, now.In(loc))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

type fakeClock struct {
	now    time.Time
	steps  []time.Duration
	waits  int
	cancel context.CancelFunc
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Wait(ctx context.Context, d time.Duration) error {
	if len(c.steps) > 0 {
		d = c.steps[0]
		c.steps = c.steps[1:]
	}
	c.now = c.now.Add(d)
	c.waits++
	if c.cancel != nil {
		c.cancel()
		return ctx.Err()
	}
	return nil
}
func TestSchedule(t *testing.T) {
	for _, tc := range []struct {
		name   string
		steps  []time.Duration
		cancel bool
	}{
		{"normal", nil, false},
		{"resume after deadline", []time.Duration{time.Hour}, false},
		{"clock moved backwards", []time.Duration{-time.Hour, time.Hour}, false},
		{"cancel", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), steps: tc.steps}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				c.cancel = cancel
			}
			called := 0
			code, err := schedule(ctx, c, c.now.Add(3*time.Second), func() int { called++; return 42 })
			if tc.cancel {
				if code != 130 || err == nil || called != 0 {
					t.Fatalf("cancel: %d %v %d", code, err, called)
				}
			} else if code != 42 || err != nil || called != 1 {
				t.Fatalf("launch: %d %v %d", code, err, called)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := schedule(ctx, &fakeClock{}, time.Time{}, func() int { t.Fatal("launched after cancellation"); return 0 }); err == nil {
		t.Fatal("missing cancellation")
	}
}
