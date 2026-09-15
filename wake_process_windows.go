package main

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var wakeKernel = syscall.NewLazyDLL("kernel32.dll")

// Mirrors JOBOBJECT_EXTENDED_LIMIT_INFORMATION, including pointer-sized fields.
type wakeJobLimits struct {
	ProcessTime, JobTime                                       int64
	Flags                                                      uint32
	MinWorkingSet, MaxWorkingSet                               uintptr
	ActiveProcesses                                            uint32
	Affinity                                                   uintptr
	Priority, Scheduling                                       uint32
	IO                                                         [6]uint64
	ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory uintptr
}

func startWakeProcess(cmd *exec.Cmd) (terminate func() error, release func(), err error) {
	job, _, callErr := wakeKernel.NewProc("CreateJobObjectW").Call(0, 0)
	if job == 0 {
		return nil, nil, fmt.Errorf("create wake job: %w", callErr)
	}
	release = func() { _ = syscall.CloseHandle(syscall.Handle(job)) }
	limits := wakeJobLimits{Flags: 0x2000} // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if ok, _, e := wakeKernel.NewProc("SetInformationJobObject").Call(job, 9, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits)); ok == 0 {
		release()
		return nil, nil, fmt.Errorf("configure wake job: %w", e)
	}
	attr := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attr = *cmd.SysProcAttr
	}
	// No child code may run (including a .cmd shim spawning Node) until assigned.
	attr.CreationFlags |= 0x4 // CREATE_SUSPENDED
	cmd.SysProcAttr = &attr
	if err := cmd.Start(); err != nil {
		release()
		return nil, nil, err
	}
	fail := func(e error) (func() error, func(), error) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		release()
		return nil, nil, e
	}
	process, e := syscall.OpenProcess(0x100|0x1, false, uint32(cmd.Process.Pid)) // SET_QUOTA | TERMINATE
	if e != nil {
		return fail(fmt.Errorf("open suspended wake process: %w", e))
	}
	ok, _, assignErr := wakeKernel.NewProc("AssignProcessToJobObject").Call(job, uintptr(process))
	_ = syscall.CloseHandle(process)
	if ok == 0 {
		return fail(fmt.Errorf("assign wake job: %w", assignErr))
	}
	if e := resumeWakeProcess(uint32(cmd.Process.Pid)); e != nil {
		return fail(e)
	}
	terminate = func() error {
		if ok, _, e := wakeKernel.NewProc("TerminateJobObject").Call(job, 1); ok == 0 {
			return e
		}
		// Termination is asynchronous; do not remove the temporary cwd until
		// every descendant has stopped using it.
		for {
			var accounting struct {
				Times                             [4]int64
				Faults, Total, Active, Terminated uint32
			}
			if ok, _, e := wakeKernel.NewProc("QueryInformationJobObject").Call(job, 1, uintptr(unsafe.Pointer(&accounting)), unsafe.Sizeof(accounting), 0); ok == 0 {
				return e
			}
			if accounting.Active == 0 {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	closeJob := release
	var once sync.Once
	return terminate, func() { once.Do(func() { _ = terminate(); closeJob() }) }, nil
}

// exec.Cmd owns the process handle and Wait, but does not expose the initial
// thread handle. A suspended new process has one thread, found through Toolhelp.
func resumeWakeProcess(pid uint32) error {
	snapshot, _, e := wakeKernel.NewProc("CreateToolhelp32Snapshot").Call(0x4, 0) // SNAPTHREAD
	if snapshot == ^uintptr(0) {
		return fmt.Errorf("snapshot wake thread: %w", e)
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))
	var entry struct {
		Size, Usage, ID, Owner uint32
		Base, Delta            int32
		Flags                  uint32
	}
	entry.Size = uint32(unsafe.Sizeof(entry))
	ok, _, e := wakeKernel.NewProc("Thread32First").Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		if entry.Owner == pid {
			thread, _, e := wakeKernel.NewProc("OpenThread").Call(0x2, 0, uintptr(entry.ID)) // SUSPEND_RESUME
			if thread == 0 {
				return fmt.Errorf("open wake thread: %w", e)
			}
			count, _, e := wakeKernel.NewProc("ResumeThread").Call(thread)
			_ = syscall.CloseHandle(syscall.Handle(thread))
			if uint32(count) == 0xffffffff {
				return fmt.Errorf("resume wake thread: %w", e)
			}
			return nil
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
		ok, _, e = wakeKernel.NewProc("Thread32Next").Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	}
	return fmt.Errorf("find suspended wake thread: %w", e)
}
