//go:build darwin

package main

import "time"

// persistentDispatch guards launchd's RunAtLoad and annual calendar rule
// before the store takes its execution lock. A removal while waiting simply
// makes the later execute observe a missing/cancelled job.
func persistentDispatch(s jobStore, id string) (skip bool, err error) {
	j, err := s.load(id)
	if err != nil {
		return false, err
	}
	wait, skip := preparePersistentDispatch(j, time.Now())
	if skip {
		return true, nil
	}
	if wait > 0 {
		time.Sleep(wait)
	}
	return false, nil
}
