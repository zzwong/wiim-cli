package wiim

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeDevice struct {
	calls             []string
	host              string
	timeout           float64
	playerStatus      map[string]any
	playerStatusCalls int
	setVolumeValues   []int
	volume            int
	volumeSet         bool
}

func (f *fakeDevice) CastInfo() (map[string]any, error) {
	return map[string]any{"name": "WiiM Ultra"}, nil
}
func (f *fakeDevice) StatusEx() (map[string]any, error) {
	return map[string]any{"project": "WiiM_Ultra", "firmware": "fw", "internet": "1"}, nil
}
func (f *fakeDevice) PlayerStatus() (map[string]any, error) {
	f.playerStatusCalls++
	if f.playerStatus != nil {
		return f.playerStatus, nil
	}
	if !f.volumeSet {
		f.volume = 38
		f.volumeSet = true
	}
	return map[string]any{"status": "stop", "vol": f.volume, "mute": "0", "mode": "49"}, nil
}
func (f *fakeDevice) MetaInfo() map[string]any {
	return map[string]any{"metaData": map[string]any{"title": "Song"}}
}
func (f *fakeDevice) Command(c string) (any, error) {
	f.calls = append(f.calls, "raw:"+c)
	return map[string]any{"command": c}, nil
}
func (f *fakeDevice) SetVolume(v int) error {
	f.calls = append(f.calls, "vol")
	f.setVolumeValues = append(f.setVolumeValues, v)
	f.volume = v
	f.volumeSet = true
	return nil
}
func (f *fakeDevice) VolumeUp(_ int) error    { f.calls = append(f.calls, "up"); return nil }
func (f *fakeDevice) VolumeDown(_ int) error  { f.calls = append(f.calls, "down"); return nil }
func (f *fakeDevice) Mute(_ bool) error       { f.calls = append(f.calls, "mute"); return nil }
func (f *fakeDevice) Playback(a string) error { f.calls = append(f.calls, "playback:"+a); return nil }
func (f *fakeDevice) PlayURL(u string) error  { f.calls = append(f.calls, "play-url:"+u); return nil }
func (f *fakeDevice) PlayM3U(u string) error  { f.calls = append(f.calls, "play-m3u:"+u); return nil }
func (f *fakeDevice) PlayPromptURL(u string) error {
	f.calls = append(f.calls, "prompt-url:"+u)
	return nil
}
func (f *fakeDevice) ClearPlaylist() error           { f.calls = append(f.calls, "clear"); return nil }
func (f *fakeDevice) Seek(_ int) error               { f.calls = append(f.calls, "seek"); return nil }
func (f *fakeDevice) PlayPreset(_ int, _ *int) error { f.calls = append(f.calls, "preset"); return nil }
func (f *fakeDevice) SwitchInput(input string) error {
	f.calls = append(f.calls, "input:"+input)
	return nil
}

func withFake(t *testing.T) (*fakeDevice, func()) {
	t.Helper()
	fd := &fakeDevice{volume: 38}
	old := newDevice
	newDevice = func(host string, timeout float64) device {
		fd.host = host
		fd.timeout = timeout
		return fd
	}
	return fd, func() { newDevice = old }
}

type fakeCastMediaStatus struct {
	calls   int
	host    string
	timeout float64
	info    CastMediaInfo
	err     error
}

func withFakeCastMediaStatus(t *testing.T) (*fakeCastMediaStatus, func()) {
	t.Helper()
	fc := &fakeCastMediaStatus{info: CastMediaInfo{App: "CastApp", Title: "Song"}}
	old := castMediaStatusFunc
	castMediaStatusFunc = func(host string, timeout float64) (CastMediaInfo, error) {
		fc.calls++
		fc.host = host
		fc.timeout = timeout
		return fc.info, fc.err
	}
	return fc, func() { castMediaStatusFunc = old }
}

func runTest(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	err := Run(args, &out, &errb)
	if err != nil {
		return ExitCode(err), out.String(), err.Error()
	}
	return 0, out.String(), errb.String()
}

func TestHelpDoesNotCreateClient(t *testing.T) {
	created := false
	old := newDevice
	newDevice = func(_ string, _ float64) device { created = true; return &fakeDevice{} }
	defer func() { newDevice = old }()
	code, out, _ := runTest("--help")
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("code %d out %s", code, out)
	}
	if !strings.Contains(out, "--host") || strings.Contains(out, "\n  -host") {
		t.Fatalf("help should show double-dash flags only: %s", out)
	}
	if created {
		t.Fatal("created client during help")
	}
}

func TestStatusJSONAllowsOptionsAfterCommand(t *testing.T) {
	_, done := withFake(t)
	defer done()
	code, out, errText := runTest("status", "--host", "1.2.3.4", "--json")
	if code != 0 {
		t.Fatalf("code %d err %s", code, errText)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		t.Fatal(err)
	}
	if data["host"] != "1.2.3.4" || data["volume"].(float64) != 38 {
		t.Fatalf("data %#v", data)
	}
}

func TestDiscoverCommandHumanAndJSON(t *testing.T) {
	done := withFakeDiscovery(t, []string{"10.0.0.1"}, map[string]*fakeDiscoveryDevice{
		"10.0.0.1": {statusEx: map[string]any{"project": "WiiM_Ultra", "firmware": "fw1"}, cast: map[string]any{"name": "WiiM Ultra"}},
	})
	defer done()

	code, out, errText := runTest("discover")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errText)
	}
	if !strings.Contains(out, "Name: WiiM Ultra") || !strings.Contains(out, "Host: 10.0.0.1") {
		t.Fatalf("unexpected human output: %q", out)
	}

	code, out, errText = runTest("--json", "discover")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errText)
	}
	var devices []DiscoveredDevice
	if err := json.Unmarshal([]byte(out), &devices); err != nil {
		t.Fatalf("not valid JSON: %v: %s", err, out)
	}
	if len(devices) != 1 || devices[0].IP != "10.0.0.1" {
		t.Fatalf("devices = %+v", devices)
	}
}

func TestDiscoverCommandDoesNotRequireHost(t *testing.T) {
	t.Setenv("WIIM_HOST", "http://ignored-host")
	done := withFakeDiscovery(t, nil, nil)
	defer done()

	code, out, errText := runTest("discover")
	if code != 0 {
		t.Fatalf("discover should not require --host: code %d err %q", code, errText)
	}
	if !strings.Contains(out, "No devices found") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestDiscoverRejectsHostAndDeviceFlagsWithoutSideEffects(t *testing.T) {
	t.Setenv("WIIM_HOST", "http://ignored-host")
	path := t.TempDir() + "/config.json"
	const config = `{"defaultHost":"legacy-host","timeout":2,"defaultDevice":"office","devices":{"office":{"host":"office-host"}}}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var ssdpCalls int
	oldSearch := ssdpSearchFunc
	ssdpSearchFunc = func(time.Duration) ([]string, error) {
		ssdpCalls++
		return nil, nil
	}
	defer func() { ssdpSearchFunc = oldSearch }()

	commands := [][]string{{"discover"}, {"device", "discover"}}
	flags := [][]string{{"--host", "cli-host"}, {"--device", "office"}}
	for _, command := range commands {
		for _, flag := range flags {
			for _, beforeCommand := range []bool{true, false} {
				for _, asJSON := range []bool{false, true} {
					args := []string{"--config", path}
					if beforeCommand {
						args = append(args, flag...)
						if asJSON {
							args = append(args, "--json")
						}
						args = append(args, command...)
					} else {
						args = append(args, command...)
						args = append(args, flag...)
						if asJSON {
							args = append(args, "--json")
						}
					}

					var out, errb bytes.Buffer
					runErr := Run(args, &out, &errb)
					if runErr == nil || ExitCode(runErr) != 2 {
						t.Fatalf("%q: error = %v, want UsageError", args, runErr)
					}
					if _, ok := runErr.(UsageError); !ok {
						t.Fatalf("%q: error type %T, want UsageError", args, runErr)
					}
					wantErr := "wiim: flag " + flag[0] + " is not valid with discover"
					if runErr.Error() != wantErr {
						t.Fatalf("%q: error = %q, want %q", args, runErr, wantErr)
					}
					if !asJSON && errb.String() != runErr.Error()+"\n" {
						t.Fatalf("%q: plain stderr = %q, want %q", args, errb.String(), runErr.Error()+"\n")
					}
					if asJSON {
						var envelope struct {
							Error struct {
								Kind     string `json:"kind"`
								Message  string `json:"message"`
								ExitCode int    `json:"exitCode"`
							} `json:"error"`
						}
						if err := json.Unmarshal(errb.Bytes(), &envelope); err != nil {
							t.Fatalf("%q: JSON stderr = %q: %v", args, errb.String(), err)
						}
						wantMessage := strings.TrimPrefix(wantErr, "wiim: ")
						if envelope.Error.Kind != "usage" || envelope.Error.Message != wantMessage || envelope.Error.ExitCode != 2 {
							t.Fatalf("%q: JSON error = %#v, want usage/%q/2", args, envelope.Error, wantMessage)
						}
					}
					after, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(after, before) {
						t.Fatalf("%q mutated config on rejection:\n%s", args, after)
					}
				}
			}
		}
	}
	if ssdpCalls != 0 {
		t.Fatalf("rejected discovery calls SSDP hook %d times", ssdpCalls)
	}
}

// TestDiscoverCommandRespectsConfigFileTimeout guards against a real gap
// found in review: runDiscover used to read a.opts.timeout directly instead
// of going through the same cliTimeout()/ResolveTimeout() path every other
// command uses, so a timeout configured in ~/.config/wiim-cli/config.json
// was silently ignored for discover specifically.
func TestDiscoverCommandRespectsConfigFileTimeout(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	var gotTimeout time.Duration
	oldSearch := ssdpSearchFunc
	ssdpSearchFunc = func(timeout time.Duration) ([]string, error) {
		gotTimeout = timeout
		return nil, nil
	}
	defer func() { ssdpSearchFunc = oldSearch }()

	cfgPath := t.TempDir() + "/config.json"
	if err := os.WriteFile(cfgPath, []byte(`{"timeout":7}`), 0600); err != nil {
		t.Fatal(err)
	}

	code, _, errText := runTest("--config", cfgPath, "discover")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errText)
	}
	if gotTimeout != 7*time.Second {
		t.Fatalf("ssdpSearchFunc got timeout %v, want 7s from config file", gotTimeout)
	}
}

// TestDiscoverCommandRejectsZeroTimeout guards against the same gap in the
// other direction: an explicit --timeout 0 is a hard usage error everywhere
// else (ResolveTimeout), but runDiscover used to silently substitute 3.0
// instead of going through that same validation.
func TestDiscoverCommandRejectsZeroTimeout(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	done := withFakeDiscovery(t, nil, nil)
	defer done()

	code, _, errText := runTest("--timeout", "0", "discover")
	if code != 2 || !strings.Contains(errText, "timeout must be") {
		t.Fatalf("code %d err %q", code, errText)
	}
}

func TestCastNowUsesDefaultTimeout(t *testing.T) {
	fake, done := withFakeCastMediaStatus(t)
	defer done()

	code, out, errText := runTest("--host", "cast.local", "cast-now")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errText)
	}
	if fake.calls != 1 || fake.host != "cast.local" || fake.timeout != 3.0 {
		t.Fatalf("capture = %+v", fake)
	}
	if !strings.Contains(out, "App: CastApp") || !strings.Contains(out, "Title: Song") {
		t.Fatalf("output %q", out)
	}
}

func TestCastNowUsesConfigTimeout(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	fake, done := withFakeCastMediaStatus(t)
	defer done()

	cfgPath := t.TempDir() + "/config.json"
	if err := os.WriteFile(cfgPath, []byte(`{"defaultHost":"cfg-host","timeout":7}`), 0600); err != nil {
		t.Fatal(err)
	}

	code, _, errText := runTest("--config", cfgPath, "cast-now")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errText)
	}
	if fake.calls != 1 || fake.host != "cfg-host" || fake.timeout != 7 {
		t.Fatalf("capture = %+v", fake)
	}
}

func TestCastNowExplicitTimeoutPrecedence(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	fake, done := withFakeCastMediaStatus(t)
	defer done()

	cfgPath := t.TempDir() + "/config.json"
	if err := os.WriteFile(cfgPath, []byte(`{"defaultHost":"cfg-host","timeout":7}`), 0600); err != nil {
		t.Fatal(err)
	}

	code, _, errText := runTest("--config", cfgPath, "--timeout", "2.5", "cast-now")
	if code != 0 {
		t.Fatalf("code %d err %q", code, errText)
	}
	if fake.calls != 1 || fake.host != "cfg-host" || fake.timeout != 2.5 {
		t.Fatalf("capture = %+v", fake)
	}
}

func TestCastNowErrorPropagation(t *testing.T) {
	fake, done := withFakeCastMediaStatus(t)
	defer done()
	fake.err = errors.New("cast boom")

	code, _, errText := runTest("--host", "cast.local", "cast-now")
	if code == 0 || !strings.Contains(errText, "cast boom") {
		t.Fatalf("code %d err %q", code, errText)
	}
	if fake.calls != 1 || fake.host != "cast.local" || fake.timeout != 3.0 {
		t.Fatalf("capture = %+v", fake)
	}
}

func TestRunWritesJSONErrorEnvelopeToStderr(t *testing.T) {
	_, done := withFake(t)
	defer done()
	var out, errb bytes.Buffer
	err := Run([]string{"--host", "1.2.3.4", "volume", "60", "--json"}, &out, &errb)
	if err == nil {
		t.Fatal("expected an error")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("ExitCode = %d, want 2", code)
	}
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal(errb.Bytes(), &envelope); jsonErr != nil {
		t.Fatalf("stderr is not valid JSON: %v\nstderr: %s", jsonErr, errb.String())
	}
	if envelope.Error.Kind != "usage" || envelope.Error.ExitCode != 2 {
		t.Fatalf("envelope = %+v", envelope)
	}
	if !strings.Contains(envelope.Error.Message, "maxVolume 55") {
		t.Fatalf("message = %q", envelope.Error.Message)
	}
}

func TestRunWritesPlainErrorToStderrWithoutJSON(t *testing.T) {
	_, done := withFake(t)
	defer done()
	var out, errb bytes.Buffer
	err := Run([]string{"--host", "1.2.3.4", "volume", "60"}, &out, &errb)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := strings.TrimSpace(errb.String()); got != err.Error() {
		t.Fatalf("stderr = %q, want %q", got, err.Error())
	}
}

func decodeErrorEnvelope(t *testing.T, stderr []byte) (kind, message string, exitCode int) {
	t.Helper()
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr, &envelope); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nstderr: %s", err, stderr)
	}
	return envelope.Error.Kind, envelope.Error.Message, envelope.Error.ExitCode
}

// TestRunHonorsJSONRegardlessOfArgOrder guards against a real bug found in
// review: the hand-rolled volume flag parser (DisableFlagParsing) returns on
// the first bad token, so --json only took effect if it appeared before
// whatever triggered the error. "volume --bogus --json" used to print plain
// text even though --json was requested.
func TestRunHonorsJSONRegardlessOfArgOrder(t *testing.T) {
	_, done := withFake(t)
	defer done()
	var out, errb bytes.Buffer
	err := Run([]string{"--host", "1.2.3.4", "volume", "--bogus", "--json"}, &out, &errb)
	if err == nil {
		t.Fatal("expected an error")
	}
	kind, message, code := decodeErrorEnvelope(t, errb.Bytes())
	if kind != "usage" || code != 2 || !strings.Contains(message, "unknown volume option") {
		t.Fatalf("kind=%q message=%q code=%d", kind, message, code)
	}
}

// TestRunHonorsJSONForCobraNativeErrors guards against a related gap: an
// unresolvable command or an unknown flag on a normal (non-volume) command
// makes cobra/pflag abort before persistent flags — including --json — are
// ever bound, so app.opts.asJSON stays false regardless of arg order. Both
// are common typo shapes a script could easily hit.
func TestRunHonorsJSONForCobraNativeErrors(t *testing.T) {
	_, done := withFake(t)
	defer done()

	var out, errb bytes.Buffer
	err := Run([]string{"bogus", "--json"}, &out, &errb)
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, _, code := decodeErrorEnvelope(t, errb.Bytes()); code != 1 {
		t.Fatalf("unknown command: exit code = %d, want 1", code)
	}

	out.Reset()
	errb.Reset()
	err = Run([]string{"status", "--bogus", "--json"}, &out, &errb)
	if err == nil {
		t.Fatal("expected an error")
	}
	if _, _, code := decodeErrorEnvelope(t, errb.Bytes()); code != 1 {
		t.Fatalf("unknown flag: exit code = %d, want 1", code)
	}
}

// TestRunDoesNotTreatJSONAfterDashDashAsARequest ensures the raw-args
// fallback respects "--" as ending flag parsing, same as the volume parser:
// a literal "--json" positional value shouldn't be misread as a JSON output
// request.
func TestRunDoesNotTreatJSONAfterDashDashAsARequest(t *testing.T) {
	_, done := withFake(t)
	defer done()
	var out, errb bytes.Buffer
	err := Run([]string{"--host", "1.2.3.4", "play-url", "--", "--json"}, &out, &errb)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := strings.TrimSpace(errb.String()); got != err.Error() {
		t.Fatalf("stderr = %q, want plain text %q (should not be JSON)", got, err.Error())
	}
}

func TestNow(t *testing.T) {
	_, done := withFake(t)
	defer done()
	code, out, _ := runTest("--host", "1.2.3.4", "now")
	if code != 0 || !strings.Contains(out, "Title: Song") {
		t.Fatalf("code %d out %s", code, out)
	}
}

func TestVolumeGetSetAndMaxVolume(t *testing.T) {
	fd, done := withFake(t)
	defer done()
	code, out, _ := runTest("--host", "1.2.3.4", "volume")
	if code != 0 || strings.TrimSpace(out) != "38" {
		t.Fatalf("code %d out %q", code, out)
	}
	code, out, _ = runTest("--host", "1.2.3.4", "volume", "30")
	if code != 0 || !strings.Contains(out, "Volume set to 30") || len(fd.setVolumeValues) == 0 || fd.setVolumeValues[len(fd.setVolumeValues)-1] != 30 {
		t.Fatalf("code %d out %q set=%v", code, out, fd.setVolumeValues)
	}
	code, _, errText := runTest("--host", "1.2.3.4", "volume", "60")
	if code != 2 || !strings.Contains(errText, "maxVolume 55") {
		t.Fatalf("code %d err %q", code, errText)
	}
	fd.volume = 38
	fd.volumeSet = false
	code, _, errText = runTest("--host", "1.2.3.4", "volume", "+20")
	if code != 2 || !strings.Contains(errText, "maxVolume 55") {
		t.Fatalf("code %d err %q", code, errText)
	}
}

func TestVolumeAllowsSignedRelativeValues(t *testing.T) {
	fd, done := withFake(t)
	defer done()

	startStatus := fd.playerStatusCalls
	startSet := len(fd.setVolumeValues)
	code, out, errText := runTest("--host", "1.2.3.4", "volume", "+5")
	if code != 0 || !strings.Contains(out, "Volume increased by 5") {
		t.Fatalf("code %d out %q err %q", code, out, errText)
	}
	if fd.playerStatusCalls != startStatus+1 || len(fd.setVolumeValues) != startSet+1 || fd.setVolumeValues[startSet] != 43 {
		t.Fatalf("status=%d set=%v calls=%#v", fd.playerStatusCalls-startStatus, fd.setVolumeValues[startSet:], fd.calls)
	}

	startStatus = fd.playerStatusCalls
	startSet = len(fd.setVolumeValues)
	code, out, errText = runTest("--host", "1.2.3.4", "volume", "-5")
	if code != 0 || !strings.Contains(out, "Volume decreased by 5") {
		t.Fatalf("code %d out %q err %q", code, out, errText)
	}
	if fd.playerStatusCalls != startStatus+1 || len(fd.setVolumeValues) != startSet+1 || fd.setVolumeValues[startSet] != 38 {
		t.Fatalf("status=%d set=%v calls=%#v", fd.playerStatusCalls-startStatus, fd.setVolumeValues[startSet:], fd.calls)
	}

	startStatus = fd.playerStatusCalls
	startSet = len(fd.setVolumeValues)
	code, out, errText = runTest("volume", "-5", "--host", "1.2.3.4")
	if code != 0 || !strings.Contains(out, "Volume decreased by 5") {
		t.Fatalf("code %d out %q err %q", code, out, errText)
	}
	if fd.playerStatusCalls != startStatus+1 || len(fd.setVolumeValues) != startSet+1 || fd.setVolumeValues[startSet] != 33 {
		t.Fatalf("status=%d set=%v calls=%#v", fd.playerStatusCalls-startStatus, fd.setVolumeValues[startSet:], fd.calls)
	}

	startStatus = fd.playerStatusCalls
	startSet = len(fd.setVolumeValues)
	code, out, errText = runTest("--host", "1.2.3.4", "volume", "--", "-5")
	if code != 0 || !strings.Contains(out, "Volume decreased by 5") {
		t.Fatalf("code %d out %q err %q", code, out, errText)
	}
	if fd.playerStatusCalls != startStatus+1 || len(fd.setVolumeValues) != startSet+1 || fd.setVolumeValues[startSet] != 28 {
		t.Fatalf("status=%d set=%v calls=%#v", fd.playerStatusCalls-startStatus, fd.setVolumeValues[startSet:], fd.calls)
	}
}

func TestRelativeVolumeAcceptsJSONNumberReportedByClient(t *testing.T) {
	fd, done := withFake(t)
	defer done()
	fd.playerStatus = map[string]any{"vol": json.Number("38")}

	code, out, errText := runTest("--host", "1.2.3.4", "volume", "+5")
	if code != 0 || !strings.Contains(out, "Volume increased by 5") || len(fd.setVolumeValues) != 1 || fd.setVolumeValues[0] != 43 {
		t.Fatalf("code %d out %q err %q set=%v", code, out, errText, fd.setVolumeValues)
	}
}

func TestVolumeGlobalFlagsBeforeAndAfterCommand(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	fd, done := withFake(t)
	defer done()
	cfgPath := t.TempDir() + "/config.json"
	if err := os.WriteFile(cfgPath, []byte(`{"defaultHost":"cfg-host","timeout":4,"maxVolume":55}`), 0600); err != nil {
		t.Fatal(err)
	}

	code, out, errText := runTest("--config", cfgPath, "volume")
	if code != 0 || strings.TrimSpace(out) != "38" || fd.host != "cfg-host" || fd.timeout != 4 {
		t.Fatalf("code %d out %q err %q host %q timeout %v", code, out, errText, fd.host, fd.timeout)
	}
	code, out, errText = runTest("volume", "--config", cfgPath)
	if code != 0 || strings.TrimSpace(out) != "38" || fd.host != "cfg-host" || fd.timeout != 4 {
		t.Fatalf("code %d out %q err %q host %q timeout %v", code, out, errText, fd.host, fd.timeout)
	}
	code, out, errText = runTest("--host", "pre-host", "--timeout", "7", "volume", "30")
	if code != 0 || !strings.Contains(out, "Volume set to 30") || fd.host != "pre-host" || fd.timeout != 7 {
		t.Fatalf("code %d out %q err %q host %q timeout %v", code, out, errText, fd.host, fd.timeout)
	}
	code, out, errText = runTest("volume", "30", "--host", "post-host", "--timeout", "8", "--json")
	if code != 0 || !strings.Contains(out, `"volume": 30`) || fd.host != "post-host" || fd.timeout != 8 {
		t.Fatalf("code %d out %q err %q host %q timeout %v", code, out, errText, fd.host, fd.timeout)
	}
}

func TestVolumeTimeoutFlagAfterCommandOverridesConfig(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	fd, done := withFake(t)
	defer done()
	cfgPath := t.TempDir() + "/config.json"
	if err := os.WriteFile(cfgPath, []byte(`{"defaultHost":"cfg-host","timeout":2,"maxVolume":55}`), 0600); err != nil {
		t.Fatal(err)
	}
	code, _, errText := runTest("volume", "30", "--config", cfgPath, "--timeout", "10")
	if code != 0 || fd.timeout != 10 {
		t.Fatalf("code %d err %q timeout %v", code, errText, fd.timeout)
	}
}

func TestVolumeHelpDoesNotCreateClient(t *testing.T) {
	created := false
	old := newDevice
	newDevice = func(_ string, _ float64) device { created = true; return &fakeDevice{} }
	defer func() { newDevice = old }()
	for _, help := range []string{"-h", "--help"} {
		code, out, errText := runTest("volume", help)
		if code != 0 || !strings.Contains(out, "get or set volume") {
			t.Fatalf("%s: code %d out %q err %q", help, code, out, errText)
		}
	}
	if created {
		t.Fatal("created client during volume help")
	}
}

func TestValidation(t *testing.T) {
	if _, _, err := parseVolume("+abc"); err == nil {
		t.Fatal("expected relative volume error")
	}
	if _, _, err := parseVolume("101"); err == nil {
		t.Fatal("expected absolute volume error")
	}
	code, _, errText := runTest("status", "--host", "https://bad")
	if code != 2 || !strings.Contains(errText, "host must be") {
		t.Fatalf("code %d err %s", code, errText)
	}
	code, _, errText = runTest("status", "--timeout", "0")
	if code != 2 || !strings.Contains(errText, "timeout must be") {
		t.Fatalf("code %d err %s", code, errText)
	}
}

func TestInputShowAndSwitch(t *testing.T) {
	fd, done := withFake(t)
	defer done()
	code, out, _ := runTest("--host", "host", "input")
	if code != 0 || strings.TrimSpace(out) != "hdmi" {
		t.Fatalf("code %d out %q", code, out)
	}
	code, out, _ = runTest("--host", "host", "input", "arc")
	if code != 0 || !strings.Contains(out, "hdmi") || fd.calls[len(fd.calls)-1] != "input:hdmi" {
		t.Fatalf("code %d out %q calls %#v", code, out, fd.calls)
	}
}

func TestURLPresetAndTransportCommands(t *testing.T) {
	fd, done := withFake(t)
	defer done()
	code, out, _ := runTest("--host", "host", "play-url", "https://example.com/a.mp3")
	if code != 0 || !strings.Contains(out, "Sent URL") || fd.calls[0] != "play-url:https://example.com/a.mp3" {
		t.Fatalf("code %d out %s calls %#v", code, out, fd.calls)
	}
	code, _, errText := runTest("--host", "host", "play-url", "file:///tmp/a.mp3")
	if code != 2 || !strings.Contains(errText, "absolute http") {
		t.Fatalf("code %d err %s", code, errText)
	}
	code, _, _ = runTest("--host", "host", "next")
	if code != 0 || fd.calls[len(fd.calls)-1] != "playback:next" {
		t.Fatalf("calls %#v", fd.calls)
	}
	code, _, _ = runTest("--host", "host", "preset", "play", "1")
	if code != 0 || fd.calls[len(fd.calls)-1] != "preset" {
		t.Fatalf("calls %#v", fd.calls)
	}
}

func TestRawOutputsJSONAndEnvHost(t *testing.T) {
	fd, done := withFake(t)
	defer done()
	t.Setenv("WIIM_HOST", "env-host")
	code, out, _ := runTest("raw", "getStatusEx")
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(out, "getStatusEx") || len(fd.calls) != 1 {
		t.Fatalf("out %s calls %#v", out, fd.calls)
	}
}

func TestSetupAndConfigSet(t *testing.T) {
	path := t.TempDir() + "/config.json"
	code, out, errText := runTest("--config", path, "setup", "--host", "wiim.local")
	if code != 0 || !strings.Contains(out, "Wrote config") {
		t.Fatalf("code %d out %s err %s", code, out, errText)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"defaultHost": "wiim.local"`) || !strings.Contains(string(data), `"maxVolume": 55`) {
		t.Fatalf("config %s", data)
	}
	code, _, errText = runTest("--config", path, "config", "set", "maxVolume", "70")
	if code != 0 {
		t.Fatalf("code %d err %s", code, errText)
	}
	cfg, err := LoadConfig(path)
	if err != nil || cfg.MaxVolume != 70 {
		t.Fatalf("cfg %#v err %v", cfg, err)
	}
	code, out, _ = runTest("--config", path, "config", "path")
	if code != 0 || strings.TrimSpace(out) != path {
		t.Fatalf("code %d path out %q", code, out)
	}
	code, _, errText = runTest("--config", path, "config", "unset", "defaultHost")
	if code != 0 {
		t.Fatalf("code %d err %s", code, errText)
	}
	cfg, _ = LoadConfig(path)
	if cfg.DefaultHost != "" {
		t.Fatalf("defaultHost should be unset: %#v", cfg)
	}
}

func TestResolveHostFromConfig(t *testing.T) {
	t.Setenv("WIIM_HOST", "")
	tmp := t.TempDir() + "/config.json"
	if err := os.WriteFile(tmp, []byte(`{"defaultHost":"cfg-host","timeout":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(tmp)
	if err != nil {
		t.Fatal(err)
	}
	host, err := ResolveHost("", "", cfg)
	if err != nil || host != "cfg-host" {
		t.Fatalf("host %s err %v", host, err)
	}
}

func TestRunRejectsInvalidCLITimeoutsBeforeExternalCalls(t *testing.T) {
	oldDevice := newDevice
	oldCast := castMediaStatusFunc
	oldSearch := ssdpSearchFunc
	defer func() {
		newDevice = oldDevice
		castMediaStatusFunc = oldCast
		ssdpSearchFunc = oldSearch
	}()

	deviceCalls, castCalls, discoveryCalls := 0, 0, 0
	newDevice = func(_ string, _ float64) device {
		deviceCalls++
		return &fakeDevice{}
	}
	castMediaStatusFunc = func(_ string, _ float64) (CastMediaInfo, error) {
		castCalls++
		return CastMediaInfo{}, nil
	}
	ssdpSearchFunc = func(_ time.Duration) ([]string, error) {
		discoveryCalls++
		return nil, nil
	}

	for _, timeout := range []string{"0", "-1", "1e-10", "NaN", "+Inf", "1e100"} {
		t.Run(timeout, func(t *testing.T) {
			for _, args := range [][]string{
				{"--host", "wiim.local", "--timeout", timeout, "status"},
				{"--host", "wiim.local", "--timeout", timeout, "cast-now"},
				{"--timeout", timeout, "discover"},
			} {
				code, _, errText := runTest(args...)
				if code != 2 || errText != "wiim: timeout must be a positive number within the supported duration range" {
					t.Fatalf("args %q: code %d err %q", args, code, errText)
				}
				if deviceCalls != 0 || castCalls != 0 || discoveryCalls != 0 {
					t.Fatalf("args %q made external calls: device=%d cast=%d discovery=%d", args, deviceCalls, castCalls, discoveryCalls)
				}
			}
		})
	}
}

func TestRunRejectsInvalidConfigTimeoutBeforeExternalCalls(t *testing.T) {
	oldDevice := newDevice
	oldCast := castMediaStatusFunc
	oldSearch := ssdpSearchFunc
	defer func() {
		newDevice = oldDevice
		castMediaStatusFunc = oldCast
		ssdpSearchFunc = oldSearch
	}()

	deviceCalls, castCalls, discoveryCalls := 0, 0, 0
	newDevice = func(_ string, _ float64) device { deviceCalls++; return &fakeDevice{} }
	castMediaStatusFunc = func(_ string, _ float64) (CastMediaInfo, error) { castCalls++; return CastMediaInfo{}, nil }
	ssdpSearchFunc = func(_ time.Duration) ([]string, error) { discoveryCalls++; return nil, nil }

	for _, timeout := range []string{"-1", "1e-10", "1e100"} {
		t.Run(timeout, func(t *testing.T) {
			path := t.TempDir() + "/config.json"
			if err := os.WriteFile(path, []byte(`{"defaultHost":"wiim.local","timeout":`+timeout+`}`), 0600); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{
				{"--config", path, "status"},
				{"--config", path, "cast-now"},
				{"--config", path, "discover"},
			} {
				code, _, errText := runTest(args...)
				if code != 2 || errText != "wiim: timeout must be a positive number within the supported duration range" {
					t.Fatalf("args %q: code %d err %q", args, code, errText)
				}
				if deviceCalls != 0 || castCalls != 0 || discoveryCalls != 0 {
					t.Fatalf("args %q made external calls: device=%d cast=%d discovery=%d", args, deviceCalls, castCalls, discoveryCalls)
				}
			}
		})
	}
}

func TestConfigSetRejectsInvalidTimeout(t *testing.T) {
	for _, timeout := range []string{"0", "-1", "1e-10", "NaN", "+Inf", "1e100"} {
		t.Run(timeout, func(t *testing.T) {
			path := t.TempDir() + "/config.json"
			code, _, errText := runTest("--config", path, "config", "set", "timeout", "--", timeout)
			if code != 2 || errText != "wiim: timeout must be a positive number within the supported duration range" {
				t.Fatalf("code %d err %q", code, errText)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("config was persisted after rejected timeout: %v", err)
			}
		})
	}
}

func TestSetupRejectsInvalidTimeoutBeforePersistence(t *testing.T) {
	for _, timeout := range []string{"0", "-1", "1e-10", "NaN", "+Inf", "1e100"} {
		t.Run(timeout, func(t *testing.T) {
			path := t.TempDir() + "/config.json"
			code, _, errText := runTest("--config", path, "--host", "wiim.local", "--timeout", timeout, "setup")
			if code != 2 || errText != "wiim: timeout must be a positive number within the supported duration range" {
				t.Fatalf("code %d err %q", code, errText)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("config was persisted after rejected timeout: %v", err)
			}
		})
	}
}

func TestRunAcceptsLargestSupportedTimeout(t *testing.T) {
	fd, done := withFake(t)
	defer done()

	timeout := math.Nextafter(maxTimeoutSeconds, 0)
	value := strconv.FormatFloat(timeout, 'g', -1, 64)
	code, _, errText := runTest("--host", "wiim.local", "--timeout", value, "status")
	if code != 0 || fd.timeout != timeout {
		t.Fatalf("code %d err %q timeout %v, want %v", code, errText, fd.timeout, timeout)
	}
}

func TestRunRejectsMalformedTimeoutWithUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--timeout", "not-a-number", "version"},
		{"volume", "--timeout", "not-a-number"},
		{"config", "set", "timeout", "not-a-number"},
	} {
		code, _, errText := runTest(args...)
		if code != 2 || errText != `wiim: invalid timeout "not-a-number"` {
			t.Fatalf("args %q: code %d err %q", args, code, errText)
		}
	}
}

func TestRunDistinguishesMissingTimeoutFromUnknownFlag(t *testing.T) {
	code, _, errText := runTest("--timeout")
	if code != 2 || errText != "wiim: flag --timeout requires a value" {
		t.Fatalf("missing timeout: code %d err %q", code, errText)
	}

	code, _, errText = runTest("--timeout-extra")
	if code != 1 || errText != "unknown flag: --timeout-extra" {
		t.Fatalf("unknown timeout flag: code %d err %q", code, errText)
	}
}

func TestRunRejectsOutOfRangeNumericTimeoutWithUsageError(t *testing.T) {
	const want = "timeout must be a positive number within the supported duration range"

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "persistent flag", args: []string{"--timeout", "1e1000", "version"}},
		{name: "volume parser", args: []string{"volume", "--timeout", "-1e1000"}},
		{name: "config set", args: []string{"--config", t.TempDir() + "/config.json", "config", "set", "timeout", "--", "1e1000"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(tc.args, &stdout, &stderr)
			if ExitCode(err) != 2 {
				t.Fatalf("exit code = %d, want 2", ExitCode(err))
			}
			usageErr, ok := err.(UsageError)
			if !ok {
				t.Fatalf("error type %T, want UsageError: %v", err, err)
			}
			if usageErr.Msg != want {
				t.Fatalf("error message = %q, want %q", usageErr.Msg, want)
			}
			if got := strings.TrimSpace(stderr.String()); got != "wiim: "+want {
				t.Fatalf("stderr = %q, want %q", got, "wiim: "+want)
			}
		})
	}
}

func TestRunRejectsExplicitInvalidTimeoutOnNonResolvingCommands(t *testing.T) {
	commands := [][]string{
		{"spotify", "logout"},
		{"config", "show"},
		{"config", "path"},
		{"config", "set", "maxVolume", "55"},
		{"config", "unset", "maxVolume"},
		{"version"},
	}
	for _, command := range commands {
		args := append([]string{"--timeout", "-1"}, command...)
		code, _, errText := runTest(args...)
		if code != 2 || errText != "wiim: timeout must be a positive number within the supported duration range" {
			t.Fatalf("args %q: code %d err %q", args, code, errText)
		}
	}
}

func TestConfigShowRejectsPersistedInvalidTimeout(t *testing.T) {
	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(`{"timeout":1e-10}`), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errText := runTest("--config", path, "config", "show")
	if code != 2 || out != "" || errText != "wiim: timeout must be a positive number within the supported duration range" {
		t.Fatalf("code %d out %q err %q", code, out, errText)
	}
}
