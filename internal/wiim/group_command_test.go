package wiim

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func groupResponse(members ...map[string]any) map[string]any {
	entries := make([]any, len(members))
	for i, member := range members {
		entries[i] = member
	}
	return map[string]any{"slaves": len(entries), "wmrm_version": "4.2", "slave_list": entries}
}

func TestDispatchGroupMembersUsesOneCommandAndFormatsPlainAndJSON(t *testing.T) {
	fd := &fakeDevice{commandValues: map[string]any{
		"multiroom:getSlaveList": groupResponse(map[string]any{"name": "Kitchen", "ip": "192.0.2.21", "volume": 20}),
	}}

	plain, err := dispatchGroup([]string{"members"}, options{}, "speaker.local", fd)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "Member 1:\nName: Kitchen\nIP: 192.0.2.21\nVolume: 20" {
		t.Fatalf("plain = %q", plain)
	}
	if got, want := strings.Join(fd.readCalls, ","), "Command:multiroom:getSlaveList"; got != want || fd.commandCalls != 1 {
		t.Fatalf("calls = %#v, command count = %d; want [%s], 1", fd.readCalls, fd.commandCalls, want)
	}
	if fd.statusExCalls != 0 || fd.castInfoCalls != 0 || fd.playerStatusCalls != 0 || fd.metaInfoCalls != 0 {
		t.Fatalf("unexpected reads: status=%d cast=%d player=%d meta=%d", fd.statusExCalls, fd.castInfoCalls, fd.playerStatusCalls, fd.metaInfoCalls)
	}

	fd = &fakeDevice{commandValues: map[string]any{"multiroom:getSlaveList": groupResponse()}}
	output, err := dispatchGroup([]string{"members"}, options{asJSON: true}, "speaker.local", fd)
	if err != nil {
		t.Fatal(err)
	}
	var group GroupMembers
	if err := json.Unmarshal([]byte(output), &group); err != nil {
		t.Fatal(err)
	}
	if group.Count != 0 || group.Members == nil || len(group.Members) != 0 {
		t.Fatalf("JSON group = %#v", group)
	}
	if got, want := strings.Join(fd.readCalls, ","), "Command:multiroom:getSlaveList"; got != want || fd.commandCalls != 1 {
		t.Fatalf("calls = %#v, command count = %d; want [%s], 1", fd.readCalls, fd.commandCalls, want)
	}
}

func TestDispatchGroupStatusUsesOnlyStatusExThenOneMemberCommand(t *testing.T) {
	fd := &fakeDevice{
		statusEx: map[string]any{"DeviceName": "Living Room", "project": "WiiM_Ultra", "firmware": "4.8", "group": 0, "GroupName": "Downstairs"},
		commandValues: map[string]any{
			"multiroom:getSlaveList": groupResponse(map[string]any{"name": "Kitchen"}),
		},
	}

	output, err := dispatchGroup([]string{"status"}, options{}, "speaker.local", fd)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Name: Living Room", "Host: speaker.local", "Role: master", "Grouped: yes", "Member count: 1"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output %q does not contain %q", output, want)
		}
	}
	if got, want := strings.Join(fd.readCalls, ","), "StatusEx,Command:multiroom:getSlaveList"; got != want || fd.statusExCalls != 1 || fd.commandCalls != 1 {
		t.Fatalf("calls = %#v, status count = %d, command count = %d; want [%s], 1, 1", fd.readCalls, fd.statusExCalls, fd.commandCalls, want)
	}
	if fd.castInfoCalls != 0 || fd.playerStatusCalls != 0 || fd.metaInfoCalls != 0 {
		t.Fatalf("unexpected reads: cast=%d player=%d meta=%d", fd.castInfoCalls, fd.playerStatusCalls, fd.metaInfoCalls)
	}

	output, err = dispatchGroup([]string{"status"}, options{asJSON: true}, "speaker.local", fd)
	if err != nil {
		t.Fatal(err)
	}
	var status GroupStatus
	if err := json.Unmarshal([]byte(output), &status); err != nil {
		t.Fatal(err)
	}
	if status.Host != "speaker.local" || status.Role != "master" || status.MemberCount != 1 {
		t.Fatalf("JSON status = %#v", status)
	}
}

func TestDispatchGroupPreservesFirstAPIAndRuntimeErrors(t *testing.T) {
	statusErr := runtimef("status failed")
	fd := &fakeDevice{statusExErr: statusErr}
	_, err := dispatchGroup([]string{"status"}, options{}, "h", fd)
	if err != statusErr {
		t.Fatalf("error = %v, want first status error %v", err, statusErr)
	}
	if got, want := strings.Join(fd.readCalls, ","), "StatusEx"; got != want || fd.commandCalls != 0 {
		t.Fatalf("calls = %#v, command count = %d; want [%s], 0", fd.readCalls, fd.commandCalls, want)
	}

	commandErr := runtimef("member request failed")
	fd = &fakeDevice{commandErr: commandErr}
	_, err = dispatchGroup([]string{"members"}, options{}, "h", fd)
	if err != commandErr {
		t.Fatalf("error = %v, want command error %v", err, commandErr)
	}
	if got, want := strings.Join(fd.readCalls, ","), "Command:multiroom:getSlaveList"; got != want || fd.commandCalls != 1 {
		t.Fatalf("calls = %#v, command count = %d; want [%s], 1", fd.readCalls, fd.commandCalls, want)
	}

	fd = &fakeDevice{commandValues: map[string]any{"multiroom:getSlaveList": map[string]any{"slaves": 1}}}
	_, err = dispatchGroup([]string{"members"}, options{}, "h", fd)
	if _, ok := err.(RuntimeError); !ok {
		t.Fatalf("error type = %T, want RuntimeError: %v", err, err)
	}
}

func TestDispatchGroupArgumentErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"status", "extra"}, {"unknown"}} {
		_, err := dispatchGroup(args, options{}, "h", &fakeDevice{})
		if _, ok := err.(UsageError); !ok {
			t.Fatalf("args %#v: error type = %T, want UsageError: %v", args, err, err)
		}
	}
}

func TestGroupCommandHelpArgumentsFlagsProfilesAndErrorEnvelope(t *testing.T) {
	fd, done := withFake(t)
	defer done()
	fd.commandValues = map[string]any{"multiroom:getSlaveList": groupResponse()}

	for _, args := range [][]string{{"group", "--help"}, {"group", "status", "--help"}, {"group", "members", "--help"}} {
		code, out, errText := runTest(args...)
		if code != 0 || errText != "" || !strings.Contains(out, "read-only") {
			t.Fatalf("%q: code=%d out=%q err=%q", args, code, out, errText)
		}
	}
	for _, args := range [][]string{{"group"}, {"group", "status", "extra"}, {"group", "members", "extra"}, {"group", "unknown"}} {
		code, _, errText := runTest(args...)
		if code == 0 || errText == "" {
			t.Fatalf("%q: code=%d err=%q, want an argument or unknown-command error", args, code, errText)
		}
	}

	path := t.TempDir() + "/config.json"
	const config = `{"defaultHost":"legacy-host","defaultDevice":"office","devices":{"office":{"host":"office-host"},"kitchen":{"host":"kitchen-host"}},"timeout":7}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args      []string
		readCalls string
		host      string
	}{
		{[]string{"--config", path, "--json", "group", "status"}, "StatusEx,Command:multiroom:getSlaveList", "office-host"},
		{[]string{"group", "members", "--config", path, "--device", "kitchen", "--json"}, "Command:multiroom:getSlaveList", "kitchen-host"},
	} {
		fd.readCalls = nil
		code, out, errText := runTest(tc.args...)
		if code != 0 || errText != "" {
			t.Fatalf("%q: code=%d out=%q err=%q", tc.args, code, out, errText)
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(out), &data); err != nil {
			t.Fatalf("%q: JSON output %q: %v", tc.args, out, err)
		}
		if got := strings.Join(fd.readCalls, ","); got != tc.readCalls {
			t.Fatalf("%q: calls = %#v, want [%s]", tc.args, fd.readCalls, tc.readCalls)
		}
		if fd.host != tc.host || fd.timeout != 7 {
			t.Fatalf("%q: target = host %q timeout %v, want %s/7", tc.args, fd.host, fd.timeout, tc.host)
		}
	}

	fd.commandErr = runtimef("group API failed")
	var out, errb bytes.Buffer
	err := Run([]string{"group", "members", "--host", "speaker.local", "--json"}, &out, &errb)
	if _, ok := err.(RuntimeError); !ok {
		t.Fatalf("error type = %T, want RuntimeError: %v", err, err)
	}
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			Message  string `json:"message"`
			ExitCode int    `json:"exitCode"`
		} `json:"error"`
	}
	if err := json.Unmarshal(errb.Bytes(), &envelope); err != nil {
		t.Fatalf("error envelope %q: %v", errb.String(), err)
	}
	if envelope.Error.Kind != "runtime" || envelope.Error.Message != "group API failed" || envelope.Error.ExitCode != 1 {
		t.Fatalf("error envelope = %#v", envelope.Error)
	}
}
