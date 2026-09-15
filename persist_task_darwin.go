//go:build darwin

package main

func newTaskScheduler() taskScheduler {
	home, err := darwinHome()
	if err != nil {
		return launchdTaskScheduler{run: runLaunchctl}
	}
	return launchdTaskScheduler{home: home, uid: darwinUID(), run: runLaunchctl}
}
