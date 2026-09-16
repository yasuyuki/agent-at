package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWakeJobHelper(t *testing.T) {
	role := os.Getenv("AGENT_AT_JOB_HELPER")
	if role == "" {
		return
	}
	if role == "descendant" {
		_, _ = os.Stdout.WriteString("ready\n")
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(3)
	}
	child := exec.Command(exe, "-test.run=^TestWakeJobHelper$")
	child.Env = append(os.Environ(), "AGENT_AT_JOB_HELPER=descendant")
	child.Stdout = os.Stdout
	if err := child.Start(); err != nil {
		os.Exit(4)
	}
	if role == "wait" {
		_ = child.Wait()
	}
	os.Exit(0)
}

func TestWakeJobDescendants(t *testing.T) {
	for _, shim := range []bool{false, true} {
		for _, natural := range []bool{false, true} {
			name := map[bool]string{false: "exe", true: "cmd"}[shim] + "/" + map[bool]string{false: "terminate", true: "release"}[natural]
			t.Run(name, func(t *testing.T) {
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				args := []string{"-test.run=^TestWakeJobHelper$"}
				if shim {
					path := filepath.Join(t.TempDir(), "agent.cmd")
					if err := os.WriteFile(path, []byte("@echo off\r\n\""+exe+"\" %*\r\n"), 0600); err != nil {
						t.Fatal(err)
					}
					exe = path
				}
				cmd, err := platformCommand(exe, args)
				if err != nil {
					t.Fatal(err)
				}
				if cmd.Env == nil {
					cmd.Env = os.Environ()
				}
				role := "wait"
				if natural {
					role = "exit"
				}
				cmd.Env = append(cmd.Env, "AGENT_AT_JOB_HELPER="+role)
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				cmd.Stdout = w
				terminate, release, err := startWakeProcess(cmd)
				w.Close()
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				ready := make([]byte, 6)
				if _, err := io.ReadFull(r, ready); err != nil {
					t.Fatal(err)
				}
				if string(ready) != "ready\n" {
					t.Fatalf("readiness %q", ready)
				}
				if !natural {
					if err := terminate(); err != nil {
						t.Fatal(err)
					}
				}
				_ = cmd.Wait()
				release()
				done := make(chan error, 1)
				go func() { _, err := io.Copy(io.Discard, r); done <- err }()
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("wake descendant still holds stdout")
				}
			})
		}
	}
}
