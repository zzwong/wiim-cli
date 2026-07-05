package wiim

import (
	"errors"
	"sort"
	"testing"
	"time"
)

type fakeDiscoveryDevice struct {
	statusEx map[string]any
	cast     map[string]any
	err      error
}

func (f *fakeDiscoveryDevice) StatusEx() (map[string]any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.statusEx, nil
}
func (f *fakeDiscoveryDevice) CastInfo() (map[string]any, error) { return f.cast, nil }

func withFakeDiscovery(t *testing.T, ips []string, devices map[string]*fakeDiscoveryDevice) func() {
	t.Helper()
	oldSearch := ssdpSearchFunc
	oldClient := newDiscoveryClient
	ssdpSearchFunc = func(time.Duration) ([]string, error) { return ips, nil }
	newDiscoveryClient = func(ip string, _ float64) discoveryDevice {
		if d, ok := devices[ip]; ok {
			return d
		}
		return &fakeDiscoveryDevice{err: errors.New("connection refused")}
	}
	return func() {
		ssdpSearchFunc = oldSearch
		newDiscoveryClient = oldClient
	}
}

func TestDiscoverFiltersNonWiimRespondersAndSortsByIP(t *testing.T) {
	devices := map[string]*fakeDiscoveryDevice{
		"10.0.0.2": {statusEx: map[string]any{"project": "WiiM_Mini", "firmware": "fw2"}, cast: map[string]any{"name": "WiiM Mini"}},
		"10.0.0.1": {statusEx: map[string]any{"project": "WiiM_Ultra", "firmware": "fw1"}, cast: map[string]any{"name": "WiiM Ultra"}},
		// 10.0.0.3 has no entry in devices, so it errors (a printer or TV that
		// answered the SSDP root-device search but isn't a WiiM device).
	}
	done := withFakeDiscovery(t, []string{"10.0.0.2", "10.0.0.3", "10.0.0.1"}, devices)
	defer done()

	found, err := Discover(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d devices, want 2: %+v", len(found), found)
	}
	if !sort.SliceIsSorted(found, func(i, j int) bool { return found[i].IP < found[j].IP }) {
		t.Fatalf("results not sorted by IP: %+v", found)
	}
	if found[0].IP != "10.0.0.1" || found[0].Name != "WiiM Ultra" || found[0].Model != "WiiM_Ultra" || found[0].Firmware != "fw1" {
		t.Fatalf("found[0] = %+v", found[0])
	}
	if found[1].IP != "10.0.0.2" || found[1].Name != "WiiM Mini" {
		t.Fatalf("found[1] = %+v", found[1])
	}
}

func TestDiscoverNoResponsesIsNotAnError(t *testing.T) {
	done := withFakeDiscovery(t, nil, nil)
	defer done()

	found, err := Discover(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %+v, want empty", found)
	}
}

func TestDiscoverFallsBackToSSIDWhenCastInfoHasNoName(t *testing.T) {
	devices := map[string]*fakeDiscoveryDevice{
		"10.0.0.5": {statusEx: map[string]any{"project": "WiiM_Pro", "ssid": "Living Room WiiM"}, cast: map[string]any{}},
	}
	done := withFakeDiscovery(t, []string{"10.0.0.5"}, devices)
	defer done()

	found, err := Discover(50 * time.Millisecond)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(found) != 1 || found[0].Name != "Living Room WiiM" {
		t.Fatalf("found = %+v", found)
	}
}

func TestSSDPSearchCompletesWithoutErrorWhenNothingResponds(t *testing.T) {
	// No WiiM/UPnP devices are expected on the network this test runs on, so
	// this exercises the real socket/timeout path without asserting on any
	// specific result — it just must not error and must not hang past the
	// deadline.
	start := time.Now()
	ips, err := ssdpSearch(150 * time.Millisecond)
	if err != nil {
		t.Fatalf("ssdpSearch: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("ssdpSearch took %v, want close to the 150ms timeout", elapsed)
	}
	t.Logf("ssdpSearch found %d responder(s) on this network: %v", len(ips), ips)
}
