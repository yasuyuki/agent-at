package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWakeCmdEmptyArgsAndLiteral(t *testing.T) {
	o := helperOptions(t)
	b, err := os.ReadFile(o.Executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.CD, "fake.exe"), b, 0700); err != nil {
		t.Fatal(err)
	}
	o.Executable = filepath.Join(o.CD, "fake & %PATH% ! ^.cmd")
	if err := os.WriteFile(o.Executable, []byte("@echo off\r\n\"%~dp0fake.exe\" %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	o.Prompt = wakePrompt(`日本語 "quoted" & | < > %PATH% ! ^ \`)
	for _, agent := range []string{"claude", "codex"} {
		o.Agent = agent
		cmd, cleanup, err := prepareWake(o)
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if code := runWake(context.Background(), cmd, time.Minute, io.Discard); code != 0 {
			t.Fatalf("exit %d: %s", code, stderr.String())
		}
		var got observation
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Args, wakeArgs(o)) || got.Input != o.Prompt {
			t.Fatalf("lost argv/literal: %+v", got)
		}
		cleanup()
		if _, err := os.Stat(cmd.Dir); !os.IsNotExist(err) {
			t.Fatal("cwd survived", err)
		}
	}
	// Existing quoting/length constraints must reject before any process starts.
	o.Model = `contains "quotes"`
	if _, cleanup, err := prepareWake(o); err == nil {
		cleanup()
		t.Fatal("weakened .cmd quoting")
	}
	o.Model = string(bytes.Repeat([]byte("x"), 8200))
	if _, cleanup, err := prepareWake(o); err == nil {
		cleanup()
		t.Fatal("ignored .cmd length")
	}
}
