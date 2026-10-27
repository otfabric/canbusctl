// SPDX-License-Identifier: MIT

package errs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/isotp"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		kind Kind
		exit int
	}{
		{context.Canceled, KindInternal, ExitInternal},
		{fmt.Errorf("x: %w", context.DeadlineExceeded), KindTimeout, ExitTimeout},
		{isotp.ErrTimeout, KindTimeout, ExitTimeout},
		{isotp.ErrOverflow, KindProtocol, ExitProtocol},
		{canbus.ErrUnsupported, KindUnsupported, ExitUnsupported},
		{canbus.ErrInterfaceDown, KindTransport, ExitTransport},
		{canbus.ErrInvalidFrame, KindInvalidInput, ExitInvalidInput},
		{canbus.ErrInvalidOption, KindInvalidInput, ExitInvalidInput},
		{candb.ErrRange, KindInvalidInput, ExitInvalidInput},
		{fs.ErrPermission, KindPermission, ExitPermission},
		{errors.New("link: operation not permitted (CAP_NET_ADMIN needed)"), KindPermission, ExitPermission},
		{canbus.ErrClosed, KindTransport, ExitTransport},
		{errors.New("boom"), KindInternal, ExitInternal},
		{New(KindSafety, CodeNeedsYes, "x"), KindSafety, ExitSafety},
	}
	for _, c := range cases {
		e := Classify(c.err)
		if e.Kind != c.kind || ExitCode(e.Kind) != c.exit || e.Error() == "" {
			t.Errorf("%v: %s %d", c.err, e.Kind, ExitCode(e.Kind))
		}
	}
	for k, code := range map[Kind]int{KindUsage: 2, KindOutput: 10, KindProtocol: 6} {
		if ExitCode(k) != code {
			t.Errorf("%s", k)
		}
	}
	w := New(KindIO(), CodeIO, "read: %w", fs.ErrNotExist)
	if !errors.Is(w, fs.ErrNotExist) || Invalid("x %d", 1).Message != "x 1" {
		t.Fatal("wrapping")
	}
}

func KindIO() Kind { return KindTransport }
