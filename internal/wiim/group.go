package wiim

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
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

	if version, present := groupField(response, "wmrm_version"); present {
		parsed, err := groupString(version, "wmrm_version")
		if err != nil {
			return GroupMembers{}, err
		}
		group.WMRMVersion = parsed
	}

	slavesValue, present := groupField(response, "slaves")
	if !present {
		return GroupMembers{}, runtimef("multiroom response missing slaves")
	}
	slaves, err := groupInt(slavesValue, "slaves")
	if err != nil {
		return GroupMembers{}, err
	}

	listValue, listPresent := groupField(response, "slave_list")
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
	group.Members = make([]GroupMember, 0, len(entries))
	for index, entry := range entries {
		member, err := normalizeGroupMember(entry, index)
		if err != nil {
			return GroupMembers{}, err
		}
		group.Members = append(group.Members, member)
	}
	group.Count = len(group.Members)
	if slaves != group.Count {
		return GroupMembers{}, runtimef("slaves=%d does not match slave_list count=%d", slaves, group.Count)
	}
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
		if value, present := groupField(memberMap, field.key); present {
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
		if value, present := groupField(memberMap, field.key); present {
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
		if value, present := groupField(memberMap, field.key); present {
			parsed, err := groupBool(value, prefix+"."+field.key)
			if err != nil {
				return GroupMember{}, err
			}
			field.set(&parsed)
		}
	}
	return member, nil
}

// groupField finds a map field without treating Linkplay's inconsistent key
// capitalization as a protocol difference. An exact key wins if both forms
// happen to be supplied.
func groupField(m map[string]any, name string) (any, bool) {
	if value, ok := m[name]; ok {
		return value, true
	}
	for key, value := range m {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
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
	if value >= float64(maxInt()) {
		return 0, runtimef("%s is out of range", field)
	}
	return int(value), nil
}

func groupDecimalInt(value, field string) (int, error) {
	integer, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
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
