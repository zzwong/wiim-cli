//go:build linux || darwin

package wiim

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWriteFileAtomicReplacesSymlinkReferent(t *testing.T) {
	baseDir := t.TempDir()
	referentDir := filepath.Join(baseDir, "referents")
	linkDir := filepath.Join(baseDir, "links")
	if err := os.MkdirAll(referentDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0700); err != nil {
		t.Fatal(err)
	}
	referent := filepath.Join(referentDir, "referent.json")
	link := filepath.Join(linkDir, "config.json")
	oldData := []byte("{\"complete\":\"old config\"}\n")
	newData := []byte("{\"complete\":\"new config\"}\n")
	if err := os.WriteFile(referent, oldData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(referent, link); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(link, newData); err != nil {
		t.Fatalf("writeFileAtomic() error = %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config path mode = %v, want symlink", info.Mode())
	}
	got, err := os.ReadFile(referent)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatalf("referent data = %q, want %q", got, newData)
	}
	if temps := configTempFiles(t, referent); len(temps) != 0 {
		t.Fatalf("temporary referent files remain: %q", temps)
	}
	if temps := configTempFiles(t, link); len(temps) != 0 {
		t.Fatalf("temporary symlink-named files remain: %q", temps)
	}
}

func TestSaveConfigThroughSymlinkPreservesReferentMetadata(t *testing.T) {
	baseDir := t.TempDir()
	referentDir := filepath.Join(baseDir, "referents")
	linkDir := filepath.Join(baseDir, "links")
	if err := os.MkdirAll(referentDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0700); err != nil {
		t.Fatal(err)
	}
	referent := filepath.Join(referentDir, "referent.json")
	link := filepath.Join(linkDir, "config.json")
	initial := []byte(`{
  "futureSetting": {"source": "referent"},
  "devices": {
    "office": {
      "host": "old-office-host",
      "futureProfileSetting": true
    }
  }
}`)
	if err := os.WriteFile(referent, initial, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(referent, link); err != nil {
		t.Fatal(err)
	}

	returnedPath, err := SaveConfig(link, Config{
		DefaultHost: "current-host",
		Devices: map[string]DeviceProfile{
			"office": {Host: "current-office-host"},
		},
	})
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if returnedPath != link {
		t.Fatalf("SaveConfig() path = %q, want requested path %q", returnedPath, link)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config path mode = %v, want symlink", info.Mode())
	}

	data, err := os.ReadFile(referent)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("referent contains invalid JSON: %v", err)
	}
	if got, want := string(fields["defaultHost"]), `"current-host"`; got != want {
		t.Fatalf("referent defaultHost = %s, want %s", got, want)
	}
	var future map[string]string
	if err := json.Unmarshal(fields["futureSetting"], &future); err != nil || future["source"] != "referent" {
		t.Fatalf("referent futureSetting = %s, %v; want preserved referent metadata", fields["futureSetting"], err)
	}
	var profiles map[string]json.RawMessage
	if err := json.Unmarshal(fields["devices"], &profiles); err != nil {
		t.Fatalf("referent devices = %s: %v", fields["devices"], err)
	}
	var office map[string]json.RawMessage
	if err := json.Unmarshal(profiles["office"], &office); err != nil {
		t.Fatalf("referent office profile = %s: %v", profiles["office"], err)
	}
	if got, want := string(office["host"]), `"current-office-host"`; got != want {
		t.Fatalf("referent office host = %s, want %s", got, want)
	}
	if got, want := string(office["futureProfileSetting"]), "true"; got != want {
		t.Fatalf("referent futureProfileSetting = %s, want %s", got, want)
	}
	if temps := configTempFiles(t, referent); len(temps) != 0 {
		t.Fatalf("temporary referent files remain: %q", temps)
	}
	if temps := configTempFiles(t, link); len(temps) != 0 {
		t.Fatalf("temporary symlink-named files remain: %q", temps)
	}
}

func TestWriteFileAtomicRejectsBrokenSymlinkWithoutChanges(t *testing.T) {
	baseDir := t.TempDir()
	referentDir := filepath.Join(baseDir, "referents")
	linkDir := filepath.Join(baseDir, "links")
	if err := os.MkdirAll(referentDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "config.json")
	brokenReferent := filepath.Join(referentDir, "missing.json")
	if err := os.Symlink(brokenReferent, link); err != nil {
		t.Fatal(err)
	}
	before, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}

	err = writeFileAtomic(link, []byte("{\"complete\":\"new config\"}\n"))
	if err == nil {
		t.Fatal("writeFileAtomic() succeeded for broken symlink")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("writeFileAtomic() error = %v, want an unresolved-target error", err)
	}
	after, readlinkErr := os.Readlink(link)
	if readlinkErr != nil {
		t.Fatal(readlinkErr)
	}
	if after != before {
		t.Fatalf("symlink target = %q, want %q", after, before)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config path mode = %v, want symlink", info.Mode())
	}
	if _, err := os.Lstat(brokenReferent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("broken referent was created: %v", err)
	}
	if temps := configTempFiles(t, link); len(temps) != 0 {
		t.Fatalf("temporary symlink-named files remain: %q", temps)
	}
	if temps := configTempFiles(t, brokenReferent); len(temps) != 0 {
		t.Fatalf("temporary referent files remain: %q", temps)
	}
}

func TestWriteFileAtomicPreservesUnixOwnership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{\"complete\":\"old config\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	dirStat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("directory ownership is unavailable")
	}
	targetInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	targetStat, ok := targetInfo.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("file ownership is unavailable")
	}

	gid := -1
	groups, err := os.Getgroups()
	if err != nil {
		t.Skipf("cannot list groups: %v", err)
	}
	for _, group := range groups {
		if group != int(dirStat.Gid) {
			gid = group
			break
		}
	}
	if gid == -1 && os.Geteuid() == 0 {
		gid = int(dirStat.Gid) + 1
	}
	if gid == -1 {
		t.Skip("no permitted group differs from the temporary file directory group")
	}
	if err := os.Chown(path, int(targetStat.Uid), gid); err != nil {
		t.Skipf("cannot assign target group for ownership preservation test: %v", err)
	}
	wantInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	wantStat, ok := wantInfo.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("file ownership is unavailable")
	}
	if int(wantStat.Gid) != gid {
		t.Skipf("target group = %d, cannot set expected group %d", wantStat.Gid, gid)
	}

	if err := writeFileAtomic(path, []byte("{\"complete\":\"new config\"}\n")); err != nil {
		t.Fatalf("writeFileAtomic() error = %v", err)
	}
	gotInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	gotStat, ok := gotInfo.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("replacement ownership is unavailable")
	}
	if gotStat.Uid != wantStat.Uid || gotStat.Gid != wantStat.Gid {
		t.Fatalf("replacement ownership = %d:%d, want %d:%d", gotStat.Uid, gotStat.Gid, wantStat.Uid, wantStat.Gid)
	}
}
