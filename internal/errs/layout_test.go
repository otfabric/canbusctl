// SPDX-License-Identifier: MIT

package errs

import (
	"fmt"
	"testing"

	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/capture"
)

// TestClassifyLayout: go-canbus v0.8.0 returns candb.ErrLayout where it
// panicked; it is an invalid database (exit 3), not an internal error.
func TestClassifyLayout(t *testing.T) {
	e := Classify(fmt.Errorf("encode: %w", candb.ErrLayout))
	if e.Kind != KindInvalidInput || e.Code != CodeInvalidDatabase || ExitCode(e.Kind) != ExitInvalidInput {
		t.Fatalf("%+v", e)
	}
}

func TestClassifyCapture(t *testing.T) {
	if e := Classify(fmt.Errorf("w: %w", capture.ErrUnrepresentable)); e.Kind != KindInvalidInput || e.Code != CodeUnrepresentable {
		t.Errorf("unrepresentable: %s/%s", e.Kind, e.Code)
	}
	if e := Classify(fmt.Errorf("r: %w", &capture.ParseError{Format: capture.FormatCandump, Msg: "bad"})); e.Kind != KindInvalidInput || e.Code != CodeInvalidCapture {
		t.Errorf("parse error: %s/%s", e.Kind, e.Code)
	}
}
