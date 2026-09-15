//go:build windows

package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestReadMapsMissingTask(t *testing.T) {
	s := windowsTaskScheduler{run: func(string, string, string) (string, error) {
		return "", &taskSchedulerError{HRESULT: hresultTaskNotFound}
	}}
	if err := s.Read(testTaskSpec()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read error = %v", err)
	}
}

func TestCreateClassifiesCollisionAndUncertainFailure(t *testing.T) {
	spec := testTaskSpec()
	collision := windowsTaskScheduler{run: func(string, string, string) (string, error) {
		return "", &taskSchedulerError{HRESULT: 0x800700b7}
	}}
	err := collision.Create(spec)
	var createErr *taskCreateError
	if !errors.As(err, &createErr) || createErr.Uncertain {
		t.Fatalf("collision error = %#v", err)
	}
	unknown := windowsTaskScheduler{run: func(string, string, string) (string, error) { return "", errors.New("transport interrupted") }}
	err = unknown.Create(spec)
	if !errors.As(err, &createErr) || !createErr.Uncertain {
		t.Fatalf("uncertain error = %#v", err)
	}
}

func TestSchedulerUsesNamesAndStructuredDefinition(t *testing.T) {
	spec := testTaskSpec()
	var operation, name, definition string
	s := windowsTaskScheduler{run: func(op, gotName, gotDefinition string) (string, error) {
		operation, name, definition = op, gotName, gotDefinition
		return "", nil
	}}
	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	if operation != "create" || name != taskName(spec) || !strings.Contains(definition, "--internal-persist "+spec.ID) {
		t.Fatalf("Create invocation = %q, %q, %q", operation, name, definition)
	}
}

func TestMalformedSchedulerResponseRemainsUncertain(t *testing.T) {
	for _, data := range []string{`{}`, `{"ok":false}`, `{"ok":true,`, `{"ok":true}garbage`, `{"ok":false,"hresult":0}`} {
		_, err := decodeTaskResponse([]byte(data), nil)
		var createErr *taskCreateError
		if !errors.As(createResult(err), &createErr) || !createErr.Uncertain {
			t.Fatalf("%s classified as definite: %v", data, err)
		}
	}
}
