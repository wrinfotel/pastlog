package cli

import (
	"bytes"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := Execute([]string{"version"}, out, errOut)
	if code != 0 {
		t.Fatalf("version exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	want := "pastlog 0.0.0-dev (commit none, date unknown)\n"
	if got := out.String(); got != want {
		t.Errorf("version output = %q, want %q", got, want)
	}
}

func TestRealErrorsExit2WithLowercaseMessage(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := Execute([]string{"version", "--no-such-flag"}, out, errOut)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should stay empty on error, got %q", out.String())
	}
	msg := errOut.String()
	if len(msg) == 0 || msg[0] >= 'A' && msg[0] <= 'Z' {
		t.Errorf("error message should be lowercase and non-empty, got %q", msg)
	}
}
