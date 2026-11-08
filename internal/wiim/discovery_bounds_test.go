package wiim

import (
	"bytes"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type blockingDiscoveryDevice struct {
	active, peak *atomic.Int32
	release      <-chan struct{}
	started      chan<- struct{}
}

func (d *blockingDiscoveryDevice) StatusEx() (map[string]any, error) {
	n := d.active.Add(1)
	for {
		old := d.peak.Load()
		if n <= old || d.peak.CompareAndSwap(old, n) {
			break
		}
	}
	d.started <- struct{}{}
	<-d.release
	d.active.Add(-1)
	return map[string]any{"project": "synthetic"}, nil
}
func (*blockingDiscoveryDevice) CastInfo() (map[string]any, error) { return nil, nil }

func TestDiscoverBoundsConcurrentValidation(t *testing.T) {
	ips := make([]string, 64)
	for i := range ips {
		ips[i] = fmt.Sprintf("192.0.2.%d", i+1)
	}
	restore := withFakeDiscovery(t, ips, nil)
	defer restore()
	var active, peak atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, len(ips))
	newDiscoveryClient = func(string, float64) discoveryDevice {
		return &blockingDiscoveryDevice{&active, &peak, release, started}
	}
	done := make(chan error, 1)
	go func() {
		found, err := Discover(time.Millisecond)
		if err == nil && len(found) != len(ips) {
			err = fmt.Errorf("processed %d candidates, want %d", len(found), len(ips))
		}
		done <- err
	}()
	for i := 0; i < 8; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			<-done
			t.Fatal("workers failed to start")
		}
	}
	// All workers are held at the same barrier; no real network is involved.
	select {
	case <-started:
		close(release)
		<-done
		t.Fatal("more than eight concurrent validators")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() > 8 {
		t.Fatalf("peak=%d", peak.Load())
	}
}

func TestDiscoverRejectsOversizedCandidateSetBeforeValidation(t *testing.T) {
	restore := withFakeDiscovery(t, make([]string, 65), nil)
	defer restore()
	calls := 0
	newDiscoveryClient = func(string, float64) discoveryDevice { calls++; return &fakeDiscoveryDevice{} }
	found, err := Discover(time.Millisecond)
	if err == nil || len(found) != 0 || calls != 0 {
		t.Fatalf("found=%v err=%v calls=%d", found, err, calls)
	}
}

func TestDiscoveryCandidateBudget(t *testing.T) {
	seen := map[string]bool{}
	var ips []string
	for i := range 64 {
		var err error
		ips, err = appendDiscoveryCandidate(ips, seen, fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
	}
	repeated, err := appendDiscoveryCandidate(ips, seen, "0")
	if err != nil || len(repeated) != 64 {
		t.Fatalf("duplicate: len=%d err=%v", len(repeated), err)
	}
	overflow, err := appendDiscoveryCandidate(ips, seen, "overflow")
	if err == nil || overflow != nil || len(seen) != 64 {
		t.Fatalf("overflow: len=%d err=%v seen=%d", len(overflow), err, len(seen))
	}
}

func TestDiscoverOverflowCommandEnvelope(t *testing.T) {
	restore := withFakeDiscovery(t, make([]string, 65), nil)
	defer restore()
	var out, errb bytes.Buffer
	err := Run([]string{"--config", t.TempDir() + "/config.json", "--json", "discover"}, &out, &errb)
	if err == nil || ExitCode(err) != 1 || out.Len() != 0 {
		t.Fatalf("err=%v out=%q", err, out.String())
	}
	kind, _, code := decodeErrorEnvelope(t, errb.Bytes())
	if kind != "runtime" || code != 1 {
		t.Fatalf("wrong envelope: %s", errb.String())
	}
}
