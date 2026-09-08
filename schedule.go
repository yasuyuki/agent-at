package main

import (
	"context"
	"fmt"
	"regexp"
	"time"
	_ "time/tzdata"
)

var atPattern = regexp.MustCompile(`^(?:\d{4}-\d{2}-\d{2}T)?\d{2}:\d{2}(?::\d{2})?$`)

// parseAt resolves a local civil time once, rejecting folds and gaps rather than
// accepting time.Date's unspecified choice at daylight-saving transitions.
func parseAt(value string, now time.Time) (time.Time, error) {
	if !atPattern.MatchString(value) {
		return time.Time{}, fmt.Errorf("invalid --at: use HH:mm[:ss] or YYYY-MM-DDTHH:mm[:ss]")
	}
	full := len(value) > 8
	layout := "15:04"
	if full {
		layout = "2006-01-02T15:04"
	}
	if len(value) == 8 || len(value) == 19 {
		layout += ":05"
	}
	civil, err := time.Parse(layout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --at: %w", err)
	}
	if !full {
		civil = time.Date(now.Year(), now.Month(), now.Day(), civil.Hour(), civil.Minute(), civil.Second(), 0, time.UTC)
		wallNow := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
		if !civil.After(wallNow) {
			civil = civil.AddDate(0, 0, 1)
		}
	}
	loc := now.Location()
	offsets := map[int]bool{}
	// Walk zone boundaries over adjacent years, including non-hour transitions
	// and date-line moves; do not assume the offset chosen by time.Date is unique.
	end := civil.AddDate(1, 0, 0)
	for t := civil.AddDate(-1, 0, 0).In(loc); !t.After(end); {
		_, offset := t.Zone()
		offsets[offset] = true
		_, next := t.ZoneBounds()
		if next.IsZero() || !next.After(t) {
			break
		}
		t = next
	}
	var matches []time.Time
	for offset := range offsets {
		candidate := civil.Add(-time.Duration(offset) * time.Second).In(loc)
		if candidate.Format("2006-01-02T15:04:05") == civil.Format("2006-01-02T15:04:05") {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		return time.Time{}, fmt.Errorf("--at is nonexistent or ambiguous in local time zone %s", loc)
	}
	if !matches[0].After(now) {
		return time.Time{}, fmt.Errorf("--at must be in the future")
	}
	return matches[0], nil
}

type clock interface {
	Now() time.Time
	Wait(context.Context, time.Duration) error
}
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now().Round(0) }
func (wallClock) Wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func await(ctx context.Context, c clock, at time.Time) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		left := at.Sub(c.Now())
		if left <= 0 {
			return nil
		}
		// Recheck wall time each second: Windows timers may exclude suspended time.
		if left > time.Second {
			left = time.Second
		}
		if err := c.Wait(ctx, left); err != nil {
			return err
		}
	}
}
func schedule(ctx context.Context, c clock, at time.Time, launch func() int) (int, error) {
	if err := await(ctx, c, at); err != nil {
		return 130, err
	}
	return launch(), nil
}
