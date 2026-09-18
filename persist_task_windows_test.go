//go:build windows

package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestReadMapsMissingTask(t *testing.T) {
	s := windowsTaskScheduler{run: func(taskRequest) (string, error) {
		return "", &taskSchedulerError{HRESULT: hresultTaskNotFound}
	}}
	if err := s.Read(testTaskSpec()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read error = %v", err)
	}
}

func TestCreateClassifiesCollisionAndUncertainFailure(t *testing.T) {
	spec := testTaskSpec()
	collision := windowsTaskScheduler{run: func(taskRequest) (string, error) {
		return "", &taskSchedulerError{HRESULT: 0x800700b7}
	}}
	err := collision.Create(spec)
	var createErr *taskCreateError
	if !errors.As(err, &createErr) || createErr.Uncertain {
		t.Fatalf("collision error = %#v", err)
	}
	unknown := windowsTaskScheduler{run: func(taskRequest) (string, error) { return "", errors.New("transport interrupted") }}
	err = unknown.Create(spec)
	if !errors.As(err, &createErr) || !createErr.Uncertain {
		t.Fatalf("uncertain error = %#v", err)
	}
}

func TestSchedulerUsesNamesAndStructuredDefinition(t *testing.T) {
	spec := testTaskSpec()
	var request taskRequest
	s := windowsTaskScheduler{run: func(got taskRequest) (string, error) {
		request = got
		return "", nil
	}}
	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	if request.Operation != "create" || request.Name != taskName(spec) || !strings.Contains(request.XML, "--internal-persist "+spec.ID) {
		t.Fatalf("Create invocation = %q, %q, %q", request.Operation, request.Name, request.XML)
	}
}

func TestCreateSendsACredentialOnlyForPasswordLogon(t *testing.T) {
	var request taskRequest
	s := windowsTaskScheduler{run: func(got taskRequest) (string, error) {
		request = got
		return "", nil
	}}
	spec := testTaskSpec()
	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	if request.LogonType != taskLogonInteractiveToken || request.User != "" || request.Password != "" {
		t.Fatalf("interactive request carried a credential: logon type %d, user %v, password %v", request.LogonType, request.User != "", request.Password != "")
	}
	spec.Password = "typed at the console"
	if err := s.Create(spec); err == nil {
		t.Fatal("accepted a password for an interactive job")
	}
	spec.Logon, spec.Password = logonPassword, ""
	if err := s.Create(spec); err == nil {
		t.Fatal("registered a password job without a password")
	}
	spec.Password = "typed at the console"
	if err := s.Create(spec); err != nil {
		t.Fatal(err)
	}
	if request.LogonType != taskLogonPassword || request.Password != spec.Password || request.User == "" {
		t.Fatalf("password request = logon type %d, user %v, password matched %v", request.LogonType, request.User != "", request.Password == spec.Password)
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
