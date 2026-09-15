//go:build !windows

package main

type unsupportedTaskScheduler struct{}

func newTaskScheduler() taskScheduler { return unsupportedTaskScheduler{} }
func (unsupportedTaskScheduler) Create(taskSpec) error {
	return &taskCreateError{Err: errTaskSchedulerUnsupported}
}
func (unsupportedTaskScheduler) Read(taskSpec) error   { return errTaskSchedulerUnsupported }
func (unsupportedTaskScheduler) Delete(taskSpec) error { return errTaskSchedulerUnsupported }
