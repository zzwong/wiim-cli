package wiim

import (
	"bytes"
	"strings"
	"testing"
)

func TestPresetListDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatchPreset([]string{"list"}, options{}, fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "raw:getPresetInfo" {
		t.Fatalf("calls %#v", fd.calls)
	}
	// fakeDevice.Command returns map[string]any{"command": "getPresetInfo"}
	// which has no preset_list key -> FormatPresets returns "No presets configured"
	if out != "No presets configured" {
		t.Fatalf("output = %q", out)
	}
}

func TestPresetListWithJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatchPreset([]string{"list"}, options{asJSON: true}, fd)
	if err != nil {
		t.Fatal(err)
	}
	// With --json, dispatchPreset calls FormatRaw directly
	if !strings.Contains(out, "getPresetInfo") {
		t.Fatalf("output should contain raw value: %s", out)
	}
}

func TestPresetPlayDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatchPreset([]string{"play", "1"}, options{}, fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "preset" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if !strings.Contains(out, "Playing preset 1") {
		t.Fatalf("output: %s", out)
	}
}

func TestPresetPlayWithIndex(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatchPreset([]string{"play", "2", "3"}, options{}, fd)
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "preset" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if !strings.Contains(out, "Playing preset 2") {
		t.Fatalf("output: %s", out)
	}
}

func TestPresetErrors(t *testing.T) {
	fd := &fakeDevice{}
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"play without number", []string{"play"}},
		{"play bad number", []string{"play", "abc"}},
		{"play zero", []string{"play", "0"}},
		{"play negative", []string{"play", "-1"}},
		{"play too many args", []string{"play", "1", "2", "3"}},
		{"unknown subcommand", []string{"bogus"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dispatchPreset(tc.args, options{}, fd)
			if err == nil {
				t.Fatal("expected error")
			}
			if _, ok := err.(UsageError); !ok {
				t.Fatalf("expected UsageError, got %T: %v", err, err)
			}
		})
	}
}

func TestRelativeVolumeClampsToMaxVolume(t *testing.T) {
	fd := &fakeDevice{}
	cfg := Config{MaxVolume: 55}
	startStatus := fd.playerStatusCalls
	_, err := dispatchVolume([]string{"volume", "+20"}, options{}, cfg, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "maxVolume") {
		t.Fatalf("error should mention maxVolume: %v", err)
	}
	if fd.playerStatusCalls != startStatus+1 {
		t.Fatalf("PlayerStatus calls = %d, want 1", fd.playerStatusCalls-startStatus)
	}
	if len(fd.setVolumeValues) != 0 {
		t.Fatalf("should not have called SetVolume: %#v", fd.setVolumeValues)
	}
	if containsCall(fd.calls, "up") || containsCall(fd.calls, "down") {
		t.Fatalf("should not have called relative volume helpers: %#v", fd.calls)
	}

	fd2 := &fakeDevice{}
	startStatus = fd2.playerStatusCalls
	out, err := dispatchVolume([]string{"volume", "+17"}, options{}, cfg, fd2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Volume increased by 17") {
		t.Fatalf("output: %s", out)
	}
	if fd2.playerStatusCalls != startStatus+1 {
		t.Fatalf("PlayerStatus calls = %d, want 1", fd2.playerStatusCalls-startStatus)
	}
	if len(fd2.setVolumeValues) != 1 || fd2.setVolumeValues[0] != 55 {
		t.Fatalf("SetVolume = %#v, want 55", fd2.setVolumeValues)
	}
	if containsCall(fd2.calls, "up") || containsCall(fd2.calls, "down") {
		t.Fatalf("should not have called relative volume helpers: %#v", fd2.calls)
	}
}

func TestRelativeVolumeDownUsesExactTarget(t *testing.T) {
	fd := &fakeDevice{}
	cfg := Config{MaxVolume: 55}
	startStatus := fd.playerStatusCalls
	out, err := dispatchVolume([]string{"volume", "-30"}, options{}, cfg, fd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Volume decreased by 30") {
		t.Fatalf("output: %s", out)
	}
	if fd.playerStatusCalls != startStatus+1 {
		t.Fatalf("PlayerStatus calls = %d, want 1", fd.playerStatusCalls-startStatus)
	}
	if len(fd.setVolumeValues) != 1 || fd.setVolumeValues[0] != 8 {
		t.Fatalf("SetVolume = %#v, want 8", fd.setVolumeValues)
	}
}

func TestRelativeVolumeDownClampsToZero(t *testing.T) {
	fd := &fakeDevice{}
	cfg := Config{MaxVolume: 55}
	startStatus := fd.playerStatusCalls
	out, err := dispatchVolume([]string{"volume", "-50"}, options{}, cfg, fd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Volume decreased by 50") {
		t.Fatalf("output: %s", out)
	}
	if fd.playerStatusCalls != startStatus+1 {
		t.Fatalf("PlayerStatus calls = %d, want 1", fd.playerStatusCalls-startStatus)
	}
	if len(fd.setVolumeValues) != 1 || fd.setVolumeValues[0] != 0 {
		t.Fatalf("SetVolume = %#v, want 0", fd.setVolumeValues)
	}
}

func TestRelativeVolumeMissingCurrentVolume(t *testing.T) {
	fd := &fakeDevice{playerStatus: map[string]any{"status": "stop", "mute": "0", "mode": "49"}}
	cfg := Config{MaxVolume: 55}
	startStatus := fd.playerStatusCalls
	_, err := dispatchVolume([]string{"volume", "+5"}, options{}, cfg, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "current volume") {
		t.Fatalf("error should mention missing volume: %v", err)
	}
	if fd.playerStatusCalls != startStatus+1 {
		t.Fatalf("PlayerStatus calls = %d, want 1", fd.playerStatusCalls-startStatus)
	}
	if len(fd.setVolumeValues) != 0 {
		t.Fatalf("should not have called SetVolume: %#v", fd.setVolumeValues)
	}
}

func TestAbsoluteVolumeClampsToMaxVolume(t *testing.T) {
	fd := &fakeDevice{}
	cfg := Config{MaxVolume: 55}
	_, err := dispatchVolume([]string{"volume", "60"}, options{}, cfg, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "maxVolume") {
		t.Fatalf("error should mention maxVolume: %v", err)
	}
	if containsCall(fd.calls, "vol") {
		t.Fatalf("should not have called SetVolume: %#v", fd.calls)
	}
}

func TestVolumeGetJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatchVolume([]string{"volume"}, options{asJSON: true}, Config{}, fd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"volume": 38`) {
		t.Fatalf("output should contain volume: %s", out)
	}
	if fd.playerStatusCalls != 1 || len(fd.setVolumeValues) != 0 {
		t.Fatalf("calls status=%d set=%#v", fd.playerStatusCalls, fd.setVolumeValues)
	}
}

func TestVolumeSetJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatchVolume([]string{"volume", "25"}, options{asJSON: true}, Config{MaxVolume: 55}, fd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"volume": 25`) {
		t.Fatalf("output should contain volume: %s", out)
	}
	if len(fd.setVolumeValues) != 1 || fd.setVolumeValues[0] != 25 || fd.playerStatusCalls != 0 {
		t.Fatalf("calls status=%d set=%#v", fd.playerStatusCalls, fd.setVolumeValues)
	}
}

func TestVolumeRelativeJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatchVolume([]string{"volume", "+10"}, options{asJSON: true}, Config{MaxVolume: 55}, fd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"volumeDelta": 10`) {
		t.Fatalf("output should contain volumeDelta: %s", out)
	}
	if fd.playerStatusCalls != 1 || len(fd.setVolumeValues) != 1 || fd.setVolumeValues[0] != 48 {
		t.Fatalf("calls status=%d set=%#v", fd.playerStatusCalls, fd.setVolumeValues)
	}
}

func TestVolumeBadParseError(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatchVolume([]string{"volume", "+abc"}, options{}, Config{}, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestVolumeGetErrorPropagation(t *testing.T) {
	errFake := &errorFakeDevice{}
	_, err := dispatchVolume([]string{"volume"}, options{}, Config{}, errFake)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device error") {
		t.Fatalf("error should propagate: %v", err)
	}
}

func TestMuteUnmuteDispatch(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantMute bool
	}{
		{"mute", "mute", true},
		{"unmute", "unmute", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fd := &fakeDevice{}
			out, err := dispatch([]string{tc.cmd}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if len(fd.calls) != 1 || fd.calls[0] != "mute" {
				t.Fatalf("calls %#v", fd.calls)
			}
			if tc.wantMute && out != "Muted" {
				t.Fatalf("output = %q, want 'Muted'", out)
			}
			if !tc.wantMute && out != "Unmuted" {
				t.Fatalf("output = %q, want 'Unmuted'", out)
			}
		})
	}
}

func TestMuteUnmuteJSON(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		wantMute bool
	}{
		{"mute json", "mute", true},
		{"unmute json", "unmute", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fd := &fakeDevice{}
			out, err := dispatch([]string{tc.cmd}, options{asJSON: true}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if len(fd.calls) != 1 || fd.calls[0] != "mute" {
				t.Fatalf("calls %#v", fd.calls)
			}
			want := `"muted": true`
			if !tc.wantMute {
				want = `"muted": false`
			}
			if !strings.Contains(out, want) {
				t.Fatalf("output should contain %s: %s", want, out)
			}
		})
	}
}

func TestMuteErrorPropagation(t *testing.T) {
	errFake := &errorFakeDevice{}
	_, err := dispatch([]string{"mute"}, options{}, "host", errFake, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device error") {
		t.Fatalf("error should propagate: %v", err)
	}
}

func TestPlayURLDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"play-url", "https://example.com/audio.mp3"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "play-url:https://example.com/audio.mp3" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if out != "Sent URL to WiiM" {
		t.Fatalf("output = %q", out)
	}
}

func TestPlayM3UDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"play-m3u", "https://example.com/playlist.m3u"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "play-m3u:https://example.com/playlist.m3u" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if out != "Sent URL to WiiM" {
		t.Fatalf("output = %q", out)
	}
}

func TestPromptURLDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"prompt-url", "https://example.com/notification.mp3"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "prompt-url:https://example.com/notification.mp3" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if out != "Sent URL to WiiM" {
		t.Fatalf("output = %q", out)
	}
}

func TestPlayURLJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"play-url", "https://example.com/a.mp3"}, options{asJSON: true}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"url": "https://example.com/a.mp3"`) {
		t.Fatalf("output should contain url: %s", out)
	}
	if !strings.Contains(out, `"command": "play-url"`) {
		t.Fatalf("output should contain command: %s", out)
	}
}

func TestPlayURLValidationError(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"play-url", "file:///tmp/audio.mp3"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "URL must be an absolute http") {
		t.Fatalf("error should mention URL format: %v", err)
	}
	if len(fd.calls) != 0 {
		t.Fatalf("should not have made device calls: %#v", fd.calls)
	}
}

func TestPlayURLMissingArgument(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"play-url"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestPlayM3UValidationError(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"play-m3u", "ftp://bad"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "URL must be an absolute http") {
		t.Fatalf("error should mention URL format: %v", err)
	}
}

func TestPromptURLValidationError(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"prompt-url", "not-a-url"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "URL must be an absolute http") {
		t.Fatalf("error should mention URL format: %v", err)
	}
}

func TestSeekDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"seek", "42"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "seek" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if !strings.Contains(out, "Seeked to 42 seconds") {
		t.Fatalf("output: %s", out)
	}
}

func TestSeekJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"seek", "99"}, options{asJSON: true}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"position": 99`) {
		t.Fatalf("output should contain position: %s", out)
	}
}

func TestSeekMissingArgument(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"seek"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestSeekInvalidArgument(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"seek", "abc"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestSeekNegativeArgument(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"seek", "-5"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestSeekErrorPropagation(t *testing.T) {
	errFake := &errorFakeDevice{}
	_, err := dispatch([]string{"seek", "10"}, options{}, "host", errFake, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device error") {
		t.Fatalf("error should propagate: %v", err)
	}
}

func TestInputShowDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"input"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	// fakeDevice.PlayerStatus returns mode: "49", InputFromPlayer maps that to "hdmi"
	if !strings.Contains(out, "hdmi") {
		t.Fatalf("output should contain input name: %s", out)
	}
}

func TestInputSwitchDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"input", "optical"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Switched input to optical") {
		t.Fatalf("output: %s", out)
	}
	if !containsCall(fd.calls, "input:optical") {
		t.Fatalf("should have called SwitchInput: %#v", fd.calls)
	}
}

func TestInputSwitchJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"input", "arc"}, options{asJSON: true}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	// NormalizeInputName("arc") returns "hdmi"
	if !strings.Contains(out, `"input": "hdmi"`) {
		t.Fatalf("output should contain input: %s", out)
	}
}

func TestInputErrorPropagation(t *testing.T) {
	errFake := &errorFakeDevice{}
	_, err := dispatch([]string{"input"}, options{}, "host", errFake, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device error") {
		t.Fatalf("error should propagate: %v", err)
	}
}

func TestClearDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"clear"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "clear" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if out != "Cleared playlist" {
		t.Fatalf("output = %q", out)
	}
}

func TestClearJSON(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"clear"}, options{asJSON: true}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"cleared": true`) {
		t.Fatalf("output should contain cleared: %s", out)
	}
}

func TestClearErrorPropagation(t *testing.T) {
	errFake := &errorFakeDevice{}
	_, err := dispatch([]string{"clear"}, options{}, "host", errFake, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device error") {
		t.Fatalf("error should propagate: %v", err)
	}
}

func TestRawDispatch(t *testing.T) {
	fd := &fakeDevice{}
	out, err := dispatch([]string{"raw", "getStatusEx"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.calls) != 1 || fd.calls[0] != "raw:getStatusEx" {
		t.Fatalf("calls %#v", fd.calls)
	}
	if !strings.Contains(out, "getStatusEx") {
		t.Fatalf("output should contain raw value: %s", out)
	}
}

func TestRawMissingArgument(t *testing.T) {
	fd := &fakeDevice{}
	_, err := dispatch([]string{"raw"}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestPlaybackCommandsDispatch(t *testing.T) {
	for _, cmd := range []string{"play", "pause", "stop", "next", "prev"} {
		t.Run(cmd, func(t *testing.T) {
			fd := &fakeDevice{}
			out, err := dispatch([]string{cmd}, options{}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if len(fd.calls) != 1 || fd.calls[0] != "playback:"+cmd {
				t.Fatalf("calls %#v", fd.calls)
			}
			expected := strings.ToUpper(cmd[:1]) + cmd[1:]
			if out != expected {
				t.Fatalf("output = %q, want %q", out, expected)
			}
		})
	}
}

func TestPlaybackCommandsJSON(t *testing.T) {
	for _, cmd := range []string{"play", "pause", "stop"} {
		t.Run(cmd, func(t *testing.T) {
			fd := &fakeDevice{}
			out, err := dispatch([]string{cmd}, options{asJSON: true}, "host", fd, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, `"playbackState": "`+cmd+`"`) {
				t.Fatalf("output should contain playbackState: %s", out)
			}
		})
	}
}

func TestPlaybackErrorPropagation(t *testing.T) {
	errFake := &errorFakeDevice{}
	_, err := dispatch([]string{"play"}, options{}, "host", errFake, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device error") {
		t.Fatalf("error should propagate: %v", err)
	}
}

func TestCastNowDispatchUsesInjectedTimeout(t *testing.T) {
	fake, done := withFakeCastMediaStatus(t)
	defer done()

	out, err := dispatch([]string{"cast-now"}, options{timeout: 9.5}, "192.0.2.1", &fakeDevice{}, Config{Timeout: 1}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || fake.host != "192.0.2.1" || fake.timeout != 9.5 {
		t.Fatalf("capture = %+v", fake)
	}
	if !strings.Contains(out, "App: CastApp") {
		t.Fatalf("output: %s", out)
	}
}

func TestCastNowDispatchErrorPropagation(t *testing.T) {
	fake, done := withFakeCastMediaStatus(t)
	defer done()
	fake.err = runtimef("cast error")

	_, err := dispatch([]string{"cast-now"}, options{timeout: 1.5}, "198.51.100.1", &fakeDevice{}, Config{Timeout: 7}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "cast error") {
		t.Fatalf("err = %v", err)
	}
	if fake.calls != 1 || fake.host != "198.51.100.1" || fake.timeout != 1.5 {
		t.Fatalf("capture = %+v", fake)
	}
}

func TestDispatchCliampErrors(t *testing.T) {
	fd := &fakeDevice{}
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"too many args", []string{"status", "extra"}},
		{"unknown subcommand", []string{"bogus"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dispatchCliamp(tc.args, options{}, fd)
			if err == nil {
				t.Fatal("expected error")
			}
			if _, ok := err.(UsageError); !ok {
				t.Fatalf("expected UsageError, got %T: %v", err, err)
			}
		})
	}
}

func TestDispatchSpotifyErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"unknown subcommand", []string{"bogus"}},
		{"credentials no subcommand", []string{"credentials"}},
		{"credentials set with extra args", []string{"credentials", "set", "extra"}},
		{"credentials import-clipboard without id/secret", []string{"credentials", "import-clipboard"}},
		{"credentials import-clipboard with too many args", []string{"credentials", "import-clipboard", "a", "b"}},
		{"credentials set-secret with extra args", []string{"credentials", "set-secret", "extra"}},
		{"credentials clear with extra args", []string{"credentials", "clear", "extra"}},
		{"credentials unknown subcommand", []string{"credentials", "bogus"}},
		{"login with extra args", []string{"login", "extra"}},
		{"logout with extra args", []string{"logout", "extra"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dispatchSpotify(tc.args, options{}, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected error")
			}
			if _, ok := err.(UsageError); !ok {
				t.Fatalf("expected UsageError for %s, got %T: %v", tc.name, err, err)
			}
		})
	}
}

func TestDispatchUnknownCommandError(t *testing.T) {
	_, err := dispatch([]string{"nonexistent"}, options{}, "host", &fakeDevice{}, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(UsageError); !ok {
		t.Fatalf("expected UsageError, got %T: %v", err, err)
	}
}

func TestDispatchErrorPropagation(t *testing.T) {
	// Test that device errors propagate - use a fake that returns errors
	errFake := &errorFakeDevice{}
	_, err := dispatch([]string{"status"}, options{}, "host", errFake, Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "device error") {
		t.Fatalf("error should propagate: %v", err)
	}
}

// errorFakeDevice simulates a device that returns errors on every method.
type errorFakeDevice struct{}

func (e *errorFakeDevice) CastInfo() (map[string]any, error) {
	return nil, runtimef("device error")
}
func (e *errorFakeDevice) StatusEx() (map[string]any, error) {
	return nil, runtimef("device error")
}
func (e *errorFakeDevice) PlayerStatus() (map[string]any, error) {
	return nil, runtimef("device error")
}
func (e *errorFakeDevice) MetaInfo() map[string]any {
	return nil
}
func (e *errorFakeDevice) Command(string) (any, error) { return nil, runtimef("device error") }
func (e *errorFakeDevice) SetVolume(int) error         { return runtimef("device error") }
func (e *errorFakeDevice) VolumeUp(int) error          { return runtimef("device error") }
func (e *errorFakeDevice) VolumeDown(int) error        { return runtimef("device error") }
func (e *errorFakeDevice) Mute(bool) error             { return runtimef("device error") }
func (e *errorFakeDevice) Playback(string) error       { return runtimef("device error") }
func (e *errorFakeDevice) PlayURL(string) error        { return runtimef("device error") }
func (e *errorFakeDevice) PlayM3U(string) error        { return runtimef("device error") }
func (e *errorFakeDevice) PlayPromptURL(string) error  { return runtimef("device error") }
func (e *errorFakeDevice) ClearPlaylist() error        { return runtimef("device error") }
func (e *errorFakeDevice) Seek(int) error              { return runtimef("device error") }
func (e *errorFakeDevice) PlayPreset(int, *int) error  { return runtimef("device error") }
func (e *errorFakeDevice) SwitchInput(string) error    { return runtimef("device error") }

// containsCall reports whether call is in the fakeDevice calls list.
func containsCall(calls []string, call string) bool {
	for _, c := range calls {
		if c == call {
			return true
		}
	}
	return false
}
