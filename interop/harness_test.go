// SPDX-License-Identifier: MIT

//go:build interop

package interop

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
)

// Images of canbus-interop InteropRelease, pinned by multi-arch index
// digest (the same as go-canbus's interop tests). Override one with
// CANBUS_INTEROP_<ADAPTER>_IMAGE (upper case).
const InteropRelease = "v0.1.0"

var defaultImages = map[string]string{
	"canutils":       "ghcr.io/otfabric/canbus-interop-canutils@sha256:c4d541a178b7bedec714136c3b01897a722a06cefad4c39d6fde3afbe8b7f6de",
	"pythoncan":      "ghcr.io/otfabric/canbus-interop-pythoncan@sha256:11178bac2eb2432acb1f9620dcb4f6945ceaacaf8472056783b776cf6efd99f9",
	"pythoncanisotp": "ghcr.io/otfabric/canbus-interop-pythoncanisotp@sha256:130e54cab8e665dd917ce577ffced6d9870d6769e2f80d8beeb87ae9ff2c2492",
	"cantools":       "ghcr.io/otfabric/canbus-interop-cantools@sha256:67265f75d15d9155e877e50e92e51c530d88dce63600df24ad993e2717fa9c93",
	"canmatrix":      "ghcr.io/otfabric/canbus-interop-canmatrix@sha256:42f14775360e86ff710496be5c1eae2397ae089ec02d2c4a6082c8dba0798878",
}

var (
	rawAdapters   = []string{"canutils", "pythoncan"}
	isotpAdapters = []string{"canutils", "pythoncanisotp"}
	dbAdapters    = []string{"cantools", "canmatrix"}
)

func image(adapter string) string {
	if v := os.Getenv("CANBUS_INTEROP_" + strings.ToUpper(adapter) + "_IMAGE"); v != "" {
		return v
	}
	return defaultImages[adapter]
}

func dockerCmd() []string {
	if v := os.Getenv("CANBUS_INTEROP_DOCKER"); v != "" {
		return strings.Fields(v)
	}
	return []string{"docker"}
}

// needVCAN returns $CANBUS_TEST_IFACE (default vcan0); missing, it fails
// with CANBUS_REQUIRE_VCAN=1 and skips otherwise.
func needVCAN(t *testing.T) string {
	t.Helper()
	name := os.Getenv("CANBUS_TEST_IFACE")
	if name == "" {
		name = "vcan0"
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil || ifi.Flags&net.FlagUp == 0 || ifi.MTU < 72 {
		msg := name + " missing, down or without CAN FD MTU"
		if os.Getenv("CANBUS_REQUIRE_VCAN") == "1" {
			t.Fatal(msg)
		}
		skipf(t, "%s", msg)
	}
	return name
}

// ---- results --------------------------------------------------------------

type outcome struct {
	Test     string `json:"test"`
	Adapter  string `json:"adapter"`
	Scenario string `json:"scenario"`
	Result   string `json:"result"` // pass, fail, skip, known-difference
	Detail   string `json:"detail,omitempty"`
}

var (
	resultsMu   sync.Mutex
	results     []outcome
	skipReasons sync.Map
)

func record(t *testing.T, adapter, scenario, result, detail string) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	results = append(results, outcome{Test: t.Name(), Adapter: adapter, Scenario: scenario, Result: result, Detail: detail})
}

// scenario runs fn as a subtest and records its outcome.
func scenario(t *testing.T, adapter, name string, fn func(t *testing.T)) {
	t.Run(adapter+"/"+name, func(t *testing.T) {
		defer func() {
			switch {
			case t.Failed():
				record(t, adapter, name, "fail", "")
			case t.Skipped():
				v, _ := skipReasons.Load(t.Name())
				s, _ := v.(string)
				record(t, adapter, name, "skip", s)
			default:
				record(t, adapter, name, "pass", "")
			}
		}()
		fn(t)
	})
}

func skipf(t *testing.T, format string, args ...any) {
	t.Helper()
	msg := fmt.Sprintf(format, args...)
	skipReasons.Store(t.Name(), msg)
	t.Skip(msg)
}

// ctlBin is the canbusctl binary under test, built by TestMain.
var ctlBin string

func TestMain(m *testing.M) {
	// ASC and BLF carry zone-less local time; the containers run in UTC.
	time.Local = time.UTC
	os.Setenv("TZ", "UTC")
	dir, err := os.MkdirTemp("", "canbusctl-interop")
	if err != nil {
		panic(err)
	}
	ctlBin = filepath.Join(dir, "canbusctl")
	if v := os.Getenv("CANBUSCTL_BIN"); v != "" {
		ctlBin = v
	} else if out, err := exec.Command("go", "build", "-o", ctlBin, "..").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build canbusctl: %v\n%s", err, out)
		os.Exit(2)
	}
	code := m.Run()
	if path := os.Getenv("CANBUS_INTEROP_REPORT"); path != "" {
		resultsMu.Lock()
		sort.SliceStable(results, func(i, j int) bool { return results[i].Test < results[j].Test })
		imgs := map[string]string{}
		for a := range defaultImages {
			imgs[a] = image(a)
		}
		b, _ := json.MarshalIndent(map[string]any{"interopRelease": InteropRelease, "images": imgs, "results": results}, "", "  ")
		resultsMu.Unlock()
		_ = os.WriteFile(path, append(b, '\n'), 0o644)
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// ---- canbusctl ------------------------------------------------------------

// ctl runs canbusctl to completion and returns stdout and the exit code.
func ctl(t *testing.T, stdin []byte, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ctlBin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		if errOut.Len() > 0 {
			t.Logf("canbusctl %s: %s", strings.Join(args, " "), strings.TrimSpace(errOut.String()))
		}
	}
	return out.String(), code
}

// bg is a canbusctl process running in the background.
type bg struct {
	cmd      *exec.Cmd
	out, err bytes.Buffer
	done     chan struct{}
	code     int
}

func ctlStart(t *testing.T, args ...string) *bg {
	t.Helper()
	p := &bg{done: make(chan struct{})}
	p.cmd = exec.Command(ctlBin, args...)
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.err
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		err := p.cmd.Wait()
		if ee, ok := err.(*exec.ExitError); ok {
			p.code = ee.ExitCode()
		}
		close(p.done)
	}()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	time.Sleep(300 * time.Millisecond) // the socket is bound by then
	return p
}

func (p *bg) wait(t *testing.T, d time.Duration) (string, int) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(d):
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Fatalf("canbusctl did not finish: %s %s", p.out.String(), p.err.String())
	}
	if p.code != 0 {
		t.Logf("canbusctl stderr: %s", strings.TrimSpace(p.err.String()))
	}
	return p.out.String(), p.code
}

// ---- containers -----------------------------------------------------------

type capabilities struct {
	Adapter    string          `json:"adapter"`
	Operations []string        `json:"operations"`
	Features   map[string]bool `json:"features"`
}

func (c *capabilities) has(op string) bool {
	for _, o := range c.Operations {
		if o == op {
			return true
		}
	}
	return false
}

var capCache sync.Map

func caps(t *testing.T, adapter string) *capabilities {
	t.Helper()
	if v, ok := capCache.Load(adapter); ok {
		return v.(*capabilities)
	}
	out, code := container(t, adapter, nil, "", "print-capabilities")
	if code != 0 {
		if os.Getenv("CANBUS_REQUIRE_INTEROP") == "1" {
			t.Fatalf("%s: image %s unusable (exit %d): %s", adapter, image(adapter), code, out)
		}
		skipf(t, "image %s not available", image(adapter))
	}
	var c capabilities
	if err := json.Unmarshal(bytes.TrimSpace(out), &c); err != nil {
		t.Fatalf("%s capabilities: %v: %s", adapter, err, out)
	}
	capCache.Store(adapter, &c)
	return &c
}

// workDir is mounted into containers at /work.
func workDir(t *testing.T) string {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	return dir
}

func dockerArgs(adapter, work string, args []string) []string {
	a := append(dockerCmd(), "run", "--rm", "-i", "--network", "host")
	if work != "" {
		a = append(a, "-v", work+":/work")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	a = append(a, "--name", "canbusctl-interop-"+hex.EncodeToString(b), image(adapter))
	return append(a, args...)
}

// container runs a container to completion and returns stdout and the
// exit code.
func container(t *testing.T, adapter string, stdin []byte, work string, args ...string) ([]byte, int) {
	t.Helper()
	argv := dockerArgs(adapter, work, args)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	code := 0
	if err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		if stderr.Len() > 0 {
			t.Logf("%s stderr: %s", adapter, strings.TrimSpace(stderr.String()))
		}
	}
	return out, code
}

// event is one line of a container operation's output.
type event struct {
	Type     string    `json:"type"`
	Frame    *frameDoc `json:"frame"`
	Received int       `json:"received"`
	Len      int       `json:"len"`
	Data     string    `json:"data"`
	Message  string    `json:"message"`
}

// proc is a container running in the background (a receiver).
type proc struct {
	adapter string
	cmd     *exec.Cmd
	mu      sync.Mutex
	events  []event
	ready   chan struct{}
	done    chan struct{}
	code    int
	stderr  bytes.Buffer
}

// start starts a container and waits for its "ready" event.
func start(t *testing.T, adapter string, args ...string) *proc {
	t.Helper()
	argv := dockerArgs(adapter, "", args)
	name := argv[len(argv)-len(args)-2]
	p := &proc{adapter: adapter, ready: make(chan struct{}), done: make(chan struct{})}
	p.cmd = exec.Command(argv[0], argv[1:]...)
	p.cmd.Stderr = &p.stderr
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			k := append(dockerCmd(), "rm", "-f", name)
			_ = exec.Command(k[0], k[1:]...).Run()
			<-p.done
		}
	})
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 1<<16), 1<<24)
		once := sync.Once{}
		for sc.Scan() {
			var e event
			if json.Unmarshal(sc.Bytes(), &e) == nil {
				p.mu.Lock()
				p.events = append(p.events, e)
				p.mu.Unlock()
				if e.Type == "ready" {
					once.Do(func() { close(p.ready) })
				}
			}
		}
		err := p.cmd.Wait()
		if ee, ok := err.(*exec.ExitError); ok {
			p.code = ee.ExitCode()
		}
		close(p.done)
	}()
	select {
	case <-p.ready:
		time.Sleep(50 * time.Millisecond)
	case <-p.done:
		t.Fatalf("%s exited (%d) before ready: %s", adapter, p.code, p.stderr.String())
	case <-time.After(60 * time.Second):
		t.Fatalf("%s not ready", adapter)
	}
	return p
}

func (p *proc) wait(t *testing.T, d time.Duration) ([]event, int) {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(d):
		t.Fatalf("%s did not finish", p.adapter)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.code != 0 && p.stderr.Len() > 0 {
		t.Logf("%s stderr: %s", p.adapter, strings.TrimSpace(p.stderr.String()))
	}
	return append([]event(nil), p.events...), p.code
}

// ---- fixtures -------------------------------------------------------------

var fixtureCache sync.Map

// fixture fetches a file of canbus-interop's fixtures/ through a database
// image (they all carry them).
func fixture(t *testing.T, path string) []byte {
	t.Helper()
	if v, ok := fixtureCache.Load(path); ok {
		return v.([]byte)
	}
	caps(t, "cantools")
	out, code := container(t, "cantools", nil, "", "print-fixture", path)
	if code != 0 {
		t.Fatalf("print-fixture %s: exit %d", path, code)
	}
	fixtureCache.Store(path, out)
	return out
}

// fixtureFile writes a fixture into dir and returns its path.
func fixtureFile(t *testing.T, dir, path string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.Base(path))
	if err := os.WriteFile(p, fixture(t, path), 0o666); err != nil {
		t.Fatal(err)
	}
	return p
}

// frameDoc is the contract's frame document; canbusctl's JSON frames use
// the same field names.
type frameDoc struct {
	ID       uint32 `json:"id"`
	Extended bool   `json:"extended"`
	FD       bool   `json:"fd"`
	BRS      bool   `json:"brs"`
	ESI      bool   `json:"esi"`
	Remote   bool   `json:"remote"`
	Error    bool   `json:"error"`
	Len      int    `json:"len"`
	Data     string `json:"data"`
}

func (d *frameDoc) frame(t *testing.T) canbus.Frame {
	t.Helper()
	var f canbus.Frame
	f.ID = canbus.ID(d.ID)
	for _, fl := range []struct {
		on bool
		f  canbus.Flags
	}{{d.Extended, canbus.FlagExtended}, {d.FD, canbus.FlagFD}, {d.BRS, canbus.FlagBRS}, {d.ESI, canbus.FlagESI},
		{d.Remote, canbus.FlagRemote}, {d.Error, canbus.FlagError}} {
		if fl.on {
			f.Flags |= fl.f
		}
	}
	data, err := hex.DecodeString(d.Data)
	if err != nil {
		t.Fatalf("bad data %q", d.Data)
	}
	copy(f.Data[:], data)
	f.Len = uint8(d.Len)
	return f
}

func toDoc(f *canbus.Frame) frameDoc {
	return frameDoc{ID: uint32(f.ID), Extended: f.IsExtended(), FD: f.IsFD(), BRS: f.Flags&canbus.FlagBRS != 0,
		ESI: f.Flags&canbus.FlagESI != 0, Remote: f.IsRemote(), Error: f.IsError(), Len: int(f.Len),
		Data: hex.EncodeToString(f.Payload())}
}

// frames parses a JSON Lines fixture of frame documents.
func frames(t *testing.T, src []byte) []canbus.Frame {
	t.Helper()
	var out []canbus.Frame
	for _, line := range bytes.Split(src, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		var d frameDoc
		if err := json.Unmarshal(line, &d); err != nil {
			t.Fatalf("fixture line %q: %v", line, err)
		}
		out = append(out, d.frame(t))
	}
	return out
}

func docsJSONL(fs []canbus.Frame) []byte {
	var b bytes.Buffer
	for i := range fs {
		j, _ := json.Marshal(toDoc(&fs[i]))
		b.Write(j)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func sameFrame(a, b *canbus.Frame) bool {
	return a.ID == b.ID && a.Flags == b.Flags && a.Len == b.Len && a.Data == b.Data
}

// jsonRecords parses canbusctl's JSON Lines output.
func jsonRecords(t *testing.T, out string) []map[string]json.RawMessage {
	t.Helper()
	var recs []map[string]json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad canbusctl line %q: %v", line, err)
		}
		recs = append(recs, m)
	}
	return recs
}

func str(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}
