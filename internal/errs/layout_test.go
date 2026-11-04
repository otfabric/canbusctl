// SPDX-License-Identifier: MIT

package errs

import (
	"fmt"
	"testing"

	"github.com/otfabric/go-canbus/candb"
)

// TestClassifyLayout: go-canbus v0.8.0 returns candb.ErrLayout where it
// panicked; it is an invalid database (exit 3), not an internal error.
func TestClassifyLayout(t *testing.T) {
	e := Classify(fmt.Errorf("encode: %w", candb.ErrLayout))
	if e.Kind != KindInvalidInput || e.Code != CodeInvalidDatabase || ExitCode(e.Kind) != ExitInvalidInput {
		t.Fatalf("%+v", e)
	}
}
