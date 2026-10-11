package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMigrateDirMovesTheOldFolder(t *testing.T) {
	home := t.TempDir()
	old, dir := filepath.Join(home, ".colony"), filepath.Join(home, ".zonn")
	os.MkdirAll(filepath.Join(old, "logs"), 0o755)
	os.WriteFile(filepath.Join(old, "config.json"), []byte(`{"node_name":"PC-1"}`), 0o644)
	os.WriteFile(filepath.Join(old, "credentials.json"), []byte(`{"node_secret":"s"}`), 0o600)
	os.WriteFile(filepath.Join(old, "logs", "worker.log"), []byte("log"), 0o644)

	got, err := migrateDir(old, dir)
	if err != nil || got != dir {
		t.Fatalf("got %q, %v", got, err)
	}
	for name, want := range map[string]string{"config.json": `{"node_name":"PC-1"}`, "credentials.json": `{"node_secret":"s"}`, "logs/worker.log": "log"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != want {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dir, "credentials.json"))
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credentials mode %v", info.Mode().Perm())
		}
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("the old folder is still there")
	}
	if _, err := os.Stat(dir + ".moving"); !os.IsNotExist(err) {
		t.Fatal("the staging folder was left behind")
	}
}

func TestMigrateDirKeepsAnExistingNewFolder(t *testing.T) {
	home := t.TempDir()
	old, dir := filepath.Join(home, ".colony"), filepath.Join(home, ".zonn")
	os.MkdirAll(old, 0o755)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(old, "config.json"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("new"), 0o644)
	if got, err := migrateDir(old, dir); err != nil || got != dir {
		t.Fatalf("got %q, %v", got, err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "config.json")); string(data) != "new" {
		t.Fatal("the new folder was overwritten")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("the old folder was removed although nothing was moved")
	}
}

func TestMigrateDirOnAFreshInstall(t *testing.T) {
	home := t.TempDir()
	if got, err := migrateDir(filepath.Join(home, ".colony"), filepath.Join(home, ".zonn")); err != nil || got != filepath.Join(home, ".zonn") {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestMigrateDirKeepsTheOldFolderWhenCopyingFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs file permissions that block reading")
	}
	home := t.TempDir()
	old, dir := filepath.Join(home, ".colony"), filepath.Join(home, ".zonn")
	os.MkdirAll(old, 0o755)
	os.WriteFile(filepath.Join(old, "config.json"), []byte("cfg"), 0o644)
	os.WriteFile(filepath.Join(old, "unreadable"), []byte("x"), 0o000)
	got, err := migrateDir(old, dir)
	if err == nil || got != old {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(old, "config.json")); err != nil {
		t.Fatal("the old folder was damaged")
	}
	for _, p := range []string{dir, dir + ".moving"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s was left behind", p)
		}
	}
}

func TestEnvFallsBackToTheOldName(t *testing.T) {
	t.Setenv("ZONN_PYTHON", "")
	t.Setenv("COLONY_PYTHON", "/old/python")
	if got := Env("ZONN_PYTHON"); got != "/old/python" {
		t.Fatalf("fallback: %q", got)
	}
	t.Setenv("ZONN_PYTHON", "/new/python")
	if got := Env("ZONN_PYTHON"); got != "/new/python" {
		t.Fatalf("new name: %q", got)
	}
}
