package wiim

import "sort"

// DeviceProfileView is the presentation form of a saved device profile.
type DeviceProfileView struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Default bool   `json:"default"`
}

// ListDeviceProfiles returns saved device profiles sorted lexicographically by name.
func ListDeviceProfiles(cfg Config) []DeviceProfileView {
	names := make([]string, 0, len(cfg.Devices))
	for name := range cfg.Devices {
		names = append(names, name)
	}
	sort.Strings(names)

	profiles := make([]DeviceProfileView, 0, len(names))
	for _, name := range names {
		profiles = append(profiles, DeviceProfileView{
			Name:    name,
			Host:    cfg.Devices[name].Host,
			Default: name == cfg.DefaultDevice,
		})
	}
	return profiles
}

// AddDeviceProfile validates and adds a saved device profile to cfg.
func AddDeviceProfile(cfg *Config, name, host string) error {
	if err := ValidateDeviceName(name); err != nil {
		return err
	}
	if err := ValidateHost(host); err != nil {
		return err
	}
	if cfg == nil {
		return usagef("config is required")
	}
	if _, exists := cfg.Devices[name]; exists {
		return usagef("device profile %q is already configured", name)
	}
	devices := make(map[string]DeviceProfile, len(cfg.Devices)+1)
	for profileName, profile := range cfg.Devices {
		devices[profileName] = profile
	}
	devices[name] = DeviceProfile{Host: host}
	cfg.Devices = devices
	return nil
}

// RemoveDeviceProfile validates and removes a saved device profile from cfg.
func RemoveDeviceProfile(cfg *Config, name string) error {
	if err := ValidateDeviceName(name); err != nil {
		return err
	}
	if cfg == nil {
		return usagef("config is required")
	}
	if _, exists := cfg.Devices[name]; !exists {
		return usagef("device profile %q is not configured", name)
	}
	devices := make(map[string]DeviceProfile, len(cfg.Devices))
	for profileName, profile := range cfg.Devices {
		devices[profileName] = profile
	}
	delete(devices, name)
	cfg.Devices = devices
	if cfg.DefaultDevice == name {
		cfg.DefaultDevice = ""
	}
	return nil
}

// UseDeviceProfile selects a saved device profile as cfg's default device.
func UseDeviceProfile(cfg *Config, name string) error {
	if err := ValidateDeviceName(name); err != nil {
		return err
	}
	if cfg == nil {
		return usagef("config is required")
	}
	if _, exists := cfg.Devices[name]; !exists {
		return usagef("device profile %q is not configured", name)
	}
	cfg.DefaultDevice = name
	return nil
}
