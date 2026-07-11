package wiim

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeGroupMembersOfficialModernPopulated(t *testing.T) {
	const response = `{
		"slaves": 1,
		"wmrm_version": "4.2",
		"slave_list": [{
			"name": "Wohnzimmer",
			"uuid": "FF31F09EFFF1D2BB4FDE2B3F",
			"ip": "192.0.2.10",
			"version": "4.2",
			"type": "A31",
			"channel": 0,
			"volume": 50,
			"mute": 0,
			"battery_percent": 100,
			"battery_charging": 0
		}]
	}`
	var value any
	if err := json.Unmarshal([]byte(response), &value); err != nil {
		t.Fatal(err)
	}

	got, err := NormalizeGroupMembers(value)
	if err != nil {
		t.Fatal(err)
	}
	if got.WMRMVersion != "4.2" || got.Count != 1 || len(got.Members) != 1 {
		t.Fatalf("group = %#v", got)
	}
	member := got.Members[0]
	if member.Name != "Wohnzimmer" || member.UUID != "FF31F09EFFF1D2BB4FDE2B3F" || member.IP != "192.0.2.10" || member.Version != "4.2" || member.Type != "A31" {
		t.Fatalf("member strings = %#v", member)
	}
	assertIntPtr(t, member.Channel, 0, "channel")
	assertIntPtr(t, member.Volume, 50, "volume")
	assertBoolPtr(t, member.Muted, false, "muted")
	assertIntPtr(t, member.BatteryPercent, 100, "battery percent")
	assertBoolPtr(t, member.BatteryCharging, false, "battery charging")
	if member.Masked != nil {
		t.Fatalf("masked = %#v, want nil", member.Masked)
	}
}

func TestNormalizeGroupMembersOfficialModernEmpty(t *testing.T) {
	for _, value := range []any{
		map[string]any{"slaves": float64(0), "wmrm_version": "4.2"},
		map[string]any{"slaves": float64(0), "wmrm_version": "4.2", "slave_list": []any{}},
	} {
		got, err := NormalizeGroupMembers(value)
		if err != nil {
			t.Fatal(err)
		}
		if got.WMRMVersion != "4.2" || got.Count != 0 || len(got.Members) != 0 {
			t.Fatalf("group = %#v", got)
		}
		if got.Members == nil {
			t.Fatal("members must be a non-nil empty slice")
		}
	}
}

func TestNormalizeGroupMembersLegacyCapitalized(t *testing.T) {
	value := map[string]any{
		"Slaves":       "1",
		"WMRM_Version": "4.2",
		"Slave_list": []any{map[string]any{
			"Name":             "Kitchen",
			"UUID":             "legacy-uuid",
			"Ip":               "192.0.2.11",
			"Version":          "3.0",
			"Type":             "A98",
			"Channel":          "2",
			"Volume":           "0",
			"Mute":             "1",
			"Battery_percent":  "0",
			"Battery_charging": "false",
			"Mask":             "0",
		}},
	}
	got, err := NormalizeGroupMembers(value)
	if err != nil {
		t.Fatal(err)
	}
	member := got.Members[0]
	if got.WMRMVersion != "4.2" || member.Name != "Kitchen" || member.IP != "192.0.2.11" {
		t.Fatalf("group = %#v", got)
	}
	assertIntPtr(t, member.Channel, 2, "channel")
	assertIntPtr(t, member.Volume, 0, "volume")
	assertBoolPtr(t, member.Muted, true, "muted")
	assertIntPtr(t, member.BatteryPercent, 0, "battery percent")
	assertBoolPtr(t, member.BatteryCharging, false, "battery charging")
	assertBoolPtr(t, member.Masked, false, "masked")
}

func TestNormalizeGroupMembersSingletonAndMixedScalars(t *testing.T) {
	value := map[string]any{
		"sLaVeS":       json.Number("1"),
		"WmRm_VeRsIoN": "5.0",
		"sLaVe_LiSt": map[string]any{
			"NaMe":             "Office",
			"cHaNnEl":          json.Number("3"),
			"VoLuMe":           "12",
			"MuTe":             true,
			"BaTtErY_pErCeNt":  float64(88),
			"BaTtErY_cHaRgInG": "1",
			"MaSk":             float64(1),
		},
	}
	got, err := NormalizeGroupMembers(value)
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || len(got.Members) != 1 || got.Members[0].Name != "Office" {
		t.Fatalf("group = %#v", got)
	}
	member := got.Members[0]
	assertIntPtr(t, member.Channel, 3, "channel")
	assertIntPtr(t, member.Volume, 12, "volume")
	assertBoolPtr(t, member.Muted, true, "muted")
	assertIntPtr(t, member.BatteryPercent, 88, "battery percent")
	assertBoolPtr(t, member.BatteryCharging, true, "battery charging")
	assertBoolPtr(t, member.Masked, true, "masked")
}

func TestNormalizeGroupMembersRejectsDuplicateNormalizedFields(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		context string
	}{
		{
			name:    "top-level field",
			value:   map[string]any{"slaves": 0, "SLAVES": 0},
			context: "duplicate field \"slaves\"",
		},
		{
			name: "member field",
			value: map[string]any{"slaves": 1, "slave_list": []any{
				map[string]any{"name": "Office", "NAME": "Kitchen"},
			}},
			context: "duplicate field \"name\"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeGroupMembers(tc.value)
			assertGroupRuntimeError(t, err, tc.context)
		})
	}
}

func TestNormalizeGroupMembersRejectsUnicodeCaseFoldDuplicate(t *testing.T) {
	_, err := NormalizeGroupMembers(map[string]any{"slaves": 0, "ſlaves": 0})
	assertGroupRuntimeError(t, err, `duplicate field "slaves"`)
}

func TestGroupIntPlatformBoundaries(t *testing.T) {
	maxIntValue := maxInt()
	maxText := strconv.FormatInt(int64(maxIntValue), 10)
	for _, value := range []any{maxText, json.Number(maxText)} {
		got, err := groupInt(value, "value")
		if err != nil || got != maxIntValue {
			t.Fatalf("groupInt(%T(%q)) = %d, %v; want %d, nil", value, value, got, err, maxIntValue)
		}
	}

	var overflow string
	if strconv.IntSize == 32 {
		overflow = "2147483648"
	} else {
		overflow = "9223372036854775808"
	}
	for _, value := range []any{overflow, json.Number(overflow)} {
		_, err := groupInt(value, "value")
		assertGroupRuntimeError(t, err, "out of range")
	}

	if strconv.IntSize == 32 {
		got, err := groupInt(float64(maxIntValue), "value")
		if err != nil || got != maxIntValue {
			t.Fatalf("groupInt(float64(MaxInt)) = %d, %v; want %d, nil", got, err, maxIntValue)
		}
	}
}

func TestNormalizeGroupMembersEnforcesMemberLimit(t *testing.T) {
	for _, value := range []any{
		map[string]any{"slaves": maxInt()},
		map[string]any{"slaves": maxInt(), "slave_list": []any{map[string]any{}}},
	} {
		_, err := NormalizeGroupMembers(value)
		assertGroupRuntimeError(t, err, "maximum group members")
	}

	overLimit := make([]any, maxGroupMembers+1)
	for i := range overLimit {
		overLimit[i] = map[string]any{}
	}
	_, err := NormalizeGroupMembers(map[string]any{"slaves": 0, "slave_list": overLimit})
	assertGroupRuntimeError(t, err, "slave_list")

	atLimit := make([]any, maxGroupMembers)
	for i := range atLimit {
		atLimit[i] = map[string]any{}
	}
	got, err := NormalizeGroupMembers(map[string]any{"slaves": maxGroupMembers, "slave_list": atLimit})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != maxGroupMembers || len(got.Members) != maxGroupMembers {
		t.Fatalf("group = %#v", got)
	}
}

func TestGroupMemberJSONRetainsZeroValuedPointers(t *testing.T) {
	zero := 0
	no := false
	member := GroupMember{
		Channel:         &zero,
		Volume:          &zero,
		Muted:           &no,
		BatteryPercent:  &zero,
		BatteryCharging: &no,
		Masked:          &no,
	}
	data, err := json.Marshal(member)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"channel":         float64(0),
		"volume":          float64(0),
		"muted":           false,
		"batteryPercent":  float64(0),
		"batteryCharging": false,
		"masked":          false,
	} {
		if value, ok := got[key]; !ok || value != want {
			t.Fatalf("JSON %s: %q = %#v, want %#v", data, key, value, want)
		}
	}
}

func TestNormalizeGroupMembersRejectsMalformedResponses(t *testing.T) {
	validMember := func(field string, value any) map[string]any {
		return map[string]any{"slaves": 1, "slave_list": []any{map[string]any{field: value}}}
	}
	cases := []struct {
		name    string
		value   any
		context string
	}{
		{"non-object top", []any{}, "object"},
		{"missing slaves", map[string]any{}, "slaves"},
		{"malformed slaves", map[string]any{"slaves": "one"}, "slaves"},
		{"negative slaves", map[string]any{"slaves": -1}, "slaves"},
		{"fractional slaves", map[string]any{"slaves": 1.5}, "slaves"},
		{"overflow slaves", map[string]any{"slaves": "999999999999999999999999"}, "slaves"},
		{"missing list for nonzero slaves", map[string]any{"slaves": 1}, "slave_list"},
		{"non-array non-object list", map[string]any{"slaves": 0, "slave_list": "bad"}, "slave_list"},
		{"nil list", map[string]any{"slaves": 0, "slave_list": nil}, "slave_list"},
		{"non-object member", map[string]any{"slaves": 1, "slave_list": []any{"bad"}}, "slave_list[0]"},
		{"wrong string member type", validMember("name", 1), "slave_list[0].name"},
		{"malformed integer member type", validMember("channel", true), "slave_list[0].channel"},
		{"fractional integer member", validMember("volume", 1.5), "slave_list[0].volume"},
		{"overflow integer member", validMember("battery_percent", json.Number("999999999999999999999999")), "slave_list[0].battery_percent"},
		{"malformed boolean member type", validMember("mute", "yes"), "slave_list[0].mute"},
		{"invalid numeric boolean", validMember("battery_charging", 2), "slave_list[0].battery_charging"},
		{"fractional boolean", validMember("mask", 0.5), "slave_list[0].mask"},
		{"count mismatch", map[string]any{"slaves": 2, "slave_list": []any{map[string]any{}}}, "slaves"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeGroupMembers(tc.value)
			assertGroupRuntimeError(t, err, tc.context)
		})
	}
}

func TestNormalizeGroupMembersRejectsFloatOverflow(t *testing.T) {
	_, err := NormalizeGroupMembers(map[string]any{"slaves": math.Inf(1)})
	assertGroupRuntimeError(t, err, "slaves")
}

func assertGroupRuntimeError(t *testing.T, err error, context string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(RuntimeError); !ok {
		t.Fatalf("error type = %T, want RuntimeError: %v", err, err)
	}
	if !strings.Contains(err.Error(), context) {
		t.Fatalf("error %q does not mention %q", err, context)
	}
	if got := ExitCode(err); got != 1 {
		t.Fatalf("ExitCode = %d, want 1", got)
	}
	var envelope struct {
		Error struct {
			Kind     string `json:"kind"`
			ExitCode int    `json:"exitCode"`
			Message  string `json:"message"`
		} `json:"error"`
	}
	if parseErr := json.Unmarshal([]byte(FormatError(err, true)), &envelope); parseErr != nil {
		t.Fatalf("FormatError JSON = %q: %v", FormatError(err, true), parseErr)
	}
	if envelope.Error.Kind != "runtime" || envelope.Error.ExitCode != 1 || !strings.Contains(envelope.Error.Message, context) {
		t.Fatalf("error envelope = %#v", envelope)
	}
}

func assertIntPtr(t *testing.T, got *int, want int, name string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %#v, want %d", name, got, want)
	}
}

func assertBoolPtr(t *testing.T, got *bool, want bool, name string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %#v, want %t", name, got, want)
	}
}
