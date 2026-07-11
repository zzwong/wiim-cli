package wiim

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// GroupMember describes a device returned by Linkplay's multiroom:getSlaveList
// command.
type GroupMember struct {
	Name            string `json:"name,omitempty"`
	UUID            string `json:"uuid,omitempty"`
	IP              string `json:"ip,omitempty"`
	Version         string `json:"version,omitempty"`
	Type            string `json:"type,omitempty"`
	Channel         *int   `json:"channel,omitempty"`
	Volume          *int   `json:"volume,omitempty"`
	Muted           *bool  `json:"muted,omitempty"`
	BatteryPercent  *int   `json:"batteryPercent,omitempty"`
	BatteryCharging *bool  `json:"batteryCharging,omitempty"`
	Masked          *bool  `json:"masked,omitempty"`
}

// GroupMembers is the normalized response from Linkplay's
// multiroom:getSlaveList command.
type GroupMembers struct {
	WMRMVersion string        `json:"wmrmVersion,omitempty"`
	Count       int           `json:"count"`
	Members     []GroupMember `json:"members"`
}

// GroupStatus describes the selected device's role in a Linkplay multiroom
// group. It combines getStatusEx identity fields with normalized group members.
type GroupStatus struct {
	Name        string `json:"name,omitempty"`
	Host        string `json:"host"`
	Model       string `json:"model,omitempty"`
	Firmware    string `json:"firmware,omitempty"`
	Role        string `json:"role"`
	Grouped     bool   `json:"grouped"`
	GroupName   string `json:"groupName,omitempty"`
	MemberCount int    `json:"memberCount"`
	WMRMVersion string `json:"wmrmVersion,omitempty"`
	MasterUUID  string `json:"masterUUID,omitempty"`
}

const maxGroupMembers = 128

// NormalizeGroupStatus derives a stable view of the selected device's
// multiroom role. getStatusEx has varied field capitalization across firmware,
// so its fields are matched using the same Unicode case-folding rules as group
// member responses. Ambiguous duplicate fields are rejected.
func NormalizeGroupStatus(host string, statusEx map[string]any, members GroupMembers) (GroupStatus, error) {
	statusMap, err := normalizeGroupMap(statusEx, "status response")
	if err != nil {
		return GroupStatus{}, err
	}

	status := GroupStatus{
		Name:        groupStatusString(statusMap, "DeviceName"),
		Host:        host,
		Model:       groupStatusString(statusMap, "project"),
		Firmware:    groupStatusString(statusMap, "firmware"),
		MemberCount: len(members.Members),
		WMRMVersion: groupStatusString(statusMap, "wmrm_version"),
		MasterUUID:  groupStatusString(statusMap, "master_uuid"),
	}
	if members.WMRMVersion != "" {
		status.WMRMVersion = members.WMRMVersion
	}

	group, groupKnown := groupStatusGroup(statusMap)
	switch {
	case groupKnown && group == 1:
		status.Role = "slave"
		status.Grouped = true
	case groupKnown && group == 0 && status.MemberCount > 0:
		status.Role = "master"
		status.Grouped = true
	case groupKnown && group == 0:
		status.Role = "standalone"
	case status.MemberCount > 0:
		status.Role = "master"
		status.Grouped = true
	default:
		status.Role = "unknown"
	}
	if status.Grouped {
		status.GroupName = groupStatusString(statusMap, "GroupName")
	}
	return status, nil
}

// groupStatusString returns a firmware identity field only when it is a
// string. A malformed optional identity field is omitted rather than rendered
// as an implementation-specific value.
func groupStatusString(status map[string]any, key string) string {
	value, present := status[canonicalizeGroupKey(key)]
	if !present {
		return ""
	}
	text, _ := value.(string)
	return text
}

// groupStatusGroup reports whether getStatusEx supplied a usable Linkplay
// group flag. Values other than 0 or 1 deliberately fall through to the
// member-based role fallback.
func groupStatusGroup(status map[string]any) (int, bool) {
	value, present := status[canonicalizeGroupKey("group")]
	if !present {
		return 0, false
	}
	group, err := groupInt(value, "group")
	if err != nil || (group != 0 && group != 1) {
		return 0, false
	}
	return group, true
}

// NormalizeGroupMembers normalizes current and legacy Linkplay multiroom
// responses. Linkplay firmware has changed both key capitalization and scalar
// types over time, so field names are matched case-insensitively and integer
// and boolean values accept their documented JSON and string encodings.
func NormalizeGroupMembers(value any) (GroupMembers, error) {
	group := GroupMembers{Members: []GroupMember{}}
	response, ok := value.(map[string]any)
	if !ok {
		return GroupMembers{}, runtimef("multiroom response must be an object")
	}
	response, err := normalizeGroupMap(response, "multiroom response")
	if err != nil {
		return GroupMembers{}, err
	}

	if version, present := response[canonicalizeGroupKey("wmrm_version")]; present {
		parsed, err := groupString(version, "wmrm_version")
		if err != nil {
			return GroupMembers{}, err
		}
		group.WMRMVersion = parsed
	}

	slavesValue, present := response[canonicalizeGroupKey("slaves")]
	if !present {
		return GroupMembers{}, runtimef("multiroom response missing slaves")
	}
	slaves, err := groupInt(slavesValue, "slaves")
	if err != nil {
		return GroupMembers{}, err
	}
	if slaves > maxGroupMembers {
		return GroupMembers{}, runtimef("slaves=%d exceeds maximum group members=%d", slaves, maxGroupMembers)
	}

	listValue, listPresent := response[canonicalizeGroupKey("slave_list")]
	if !listPresent {
		if slaves != 0 {
			return GroupMembers{}, runtimef("multiroom response missing slave_list for slaves=%d", slaves)
		}
		return group, nil
	}

	entries, err := groupMemberEntries(listValue)
	if err != nil {
		return GroupMembers{}, err
	}
	if len(entries) > maxGroupMembers {
		return GroupMembers{}, runtimef("slave_list count=%d exceeds maximum group members=%d", len(entries), maxGroupMembers)
	}
	if slaves != len(entries) {
		return GroupMembers{}, runtimef("slaves=%d does not match slave_list count=%d", slaves, len(entries))
	}
	group.Members = make([]GroupMember, 0, len(entries))
	for index, entry := range entries {
		member, err := normalizeGroupMember(entry, index)
		if err != nil {
			return GroupMembers{}, err
		}
		group.Members = append(group.Members, member)
	}
	group.Count = len(group.Members)
	return group, nil
}

func groupMemberEntries(value any) ([]any, error) {
	switch list := value.(type) {
	case []any:
		return list, nil
	case map[string]any:
		return []any{list}, nil
	default:
		return nil, runtimef("slave_list must be an array or object")
	}
}

func normalizeGroupMember(value any, index int) (GroupMember, error) {
	memberMap, ok := value.(map[string]any)
	if !ok {
		return GroupMember{}, runtimef("slave_list[%d] must be an object", index)
	}
	prefix := fmt.Sprintf("slave_list[%d]", index)
	memberMap, err := normalizeGroupMap(memberMap, prefix)
	if err != nil {
		return GroupMember{}, err
	}
	member := GroupMember{}

	for _, field := range []struct {
		key string
		set func(string)
	}{
		{"name", func(v string) { member.Name = v }},
		{"uuid", func(v string) { member.UUID = v }},
		{"ip", func(v string) { member.IP = v }},
		{"version", func(v string) { member.Version = v }},
		{"type", func(v string) { member.Type = v }},
	} {
		if value, present := memberMap[canonicalizeGroupKey(field.key)]; present {
			parsed, err := groupString(value, prefix+"."+field.key)
			if err != nil {
				return GroupMember{}, err
			}
			field.set(parsed)
		}
	}

	for _, field := range []struct {
		key string
		set func(*int)
	}{
		{"channel", func(v *int) { member.Channel = v }},
		{"volume", func(v *int) { member.Volume = v }},
		{"battery_percent", func(v *int) { member.BatteryPercent = v }},
	} {
		if value, present := memberMap[canonicalizeGroupKey(field.key)]; present {
			parsed, err := groupInt(value, prefix+"."+field.key)
			if err != nil {
				return GroupMember{}, err
			}
			field.set(&parsed)
		}
	}

	for _, field := range []struct {
		key string
		set func(*bool)
	}{
		{"mute", func(v *bool) { member.Muted = v }},
		{"battery_charging", func(v *bool) { member.BatteryCharging = v }},
		{"mask", func(v *bool) { member.Masked = v }},
	} {
		if value, present := memberMap[canonicalizeGroupKey(field.key)]; present {
			parsed, err := groupBool(value, prefix+"."+field.key)
			if err != nil {
				return GroupMember{}, err
			}
			field.set(&parsed)
		}
	}
	return member, nil
}

// canonicalizeGroupKey returns the minimum rune in each rune's
// unicode.SimpleFold cycle. This produces one representative for each
// Unicode simple-fold equivalence class, matching strings.EqualFold without
// applying Unicode lowercasing or locale-specific case mappings.
func canonicalizeGroupKey(value string) string {
	return strings.Map(func(r rune) rune {
		canonical := r
		for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
			if folded < canonical {
				canonical = folded
			}
		}
		return canonical
	}, value)
}

// normalizeGroupMap canonicalizes a Linkplay object once so all later field
// lookups are deterministic. Case variants of the same field are ambiguous.
func normalizeGroupMap(m map[string]any, context string) (map[string]any, error) {
	normalized := make(map[string]any, len(m))
	for key, value := range m {
		canonicalKey := canonicalizeGroupKey(key)
		if _, exists := normalized[canonicalKey]; exists {
			return nil, runtimef("%s has duplicate field %q", context, canonicalKey)
		}
		normalized[canonicalKey] = value
	}
	return normalized, nil
}

func groupString(value any, field string) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", runtimef("%s must be a string", field)
	}
	return text, nil
}

func groupInt(value any, field string) (int, error) {
	var integer int64
	switch number := value.(type) {
	case int:
		if number < 0 {
			return 0, runtimef("%s must be a non-negative integer", field)
		}
		return number, nil
	case int8:
		integer = int64(number)
	case int16:
		integer = int64(number)
	case int32:
		integer = int64(number)
	case int64:
		integer = number
	case float32:
		return groupFloatInt(float64(number), field)
	case float64:
		return groupFloatInt(number, field)
	case json.Number:
		return groupDecimalInt(string(number), field)
	case string:
		return groupDecimalInt(number, field)
	default:
		return 0, runtimef("%s must be a non-negative integer", field)
	}
	if integer < 0 {
		return 0, runtimef("%s must be a non-negative integer", field)
	}
	if integer > int64(maxInt()) {
		return 0, runtimef("%s is out of range", field)
	}
	return int(integer), nil
}

func groupFloatInt(value float64, field string) (int, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.Trunc(value) != value {
		return 0, runtimef("%s must be a non-negative integer", field)
	}
	if value > float64(maxInt()) || (strconv.IntSize == 64 && value >= float64(maxInt())) {
		return 0, runtimef("%s is out of range", field)
	}
	integer := int(value)
	if float64(integer) != value {
		return 0, runtimef("%s is out of range", field)
	}
	return integer, nil
}

func groupDecimalInt(value, field string) (int, error) {
	integer, err := strconv.ParseInt(value, 10, strconv.IntSize)
	if err != nil {
		if strings.HasPrefix(value, "-") {
			return 0, runtimef("%s must be a non-negative integer", field)
		}
		if errors.Is(err, strconv.ErrRange) {
			return 0, runtimef("%s is out of range", field)
		}
		return 0, runtimef("%s must be a non-negative integer", field)
	}
	if integer < 0 {
		return 0, runtimef("%s must be a non-negative integer", field)
	}
	return int(integer), nil
}

func groupBool(value any, field string) (bool, error) {
	switch boolean := value.(type) {
	case bool:
		return boolean, nil
	case string:
		switch strings.ToLower(boolean) {
		case "0", "false":
			return false, nil
		case "1", "true":
			return true, nil
		}
	case int:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case int8:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case int16:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case int32:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case int64:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case uint:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case uint8:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case uint16:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case uint32:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case uint64:
		if boolean == 0 {
			return false, nil
		}
		if boolean == 1 {
			return true, nil
		}
	case float32:
		return groupFloatBool(float64(boolean), field)
	case float64:
		return groupFloatBool(boolean, field)
	case json.Number:
		return groupDecimalBool(string(boolean), field)
	}
	return false, runtimef("%s must be a boolean (0 or 1)", field)
}

func groupFloatBool(value float64, field string) (bool, error) {
	if value == 0 {
		return false, nil
	}
	if value == 1 {
		return true, nil
	}
	return false, runtimef("%s must be a boolean (0 or 1)", field)
}

func groupDecimalBool(value, field string) (bool, error) {
	if value == "0" {
		return false, nil
	}
	if value == "1" {
		return true, nil
	}
	return false, runtimef("%s must be a boolean (0 or 1)", field)
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
