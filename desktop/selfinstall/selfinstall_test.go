package selfinstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"swcode/desktop/selfinstall"
)

func TestTheAppLivesInThePersonsProgramsFolder(t *testing.T) {
	target := selfinstall.Target(filepath.Join("home", "AppData", "Local"))
	if target != filepath.Join("home", "AppData", "Local", "Programs", "swcode", "swcode-desktop.exe") {
		t.Fatalf("target %s", target)
	}
	if !selfinstall.Installed(filepath.Join("home", "appdata", "local", "programs", "SWCODE", "swcode-desktop.exe"), target) {
		t.Fatal("the same path in other letters is the installed program")
	}
	if selfinstall.Installed(filepath.Join("home", "Downloads", "swcode-desktop.exe"), target) {
		t.Fatal("a download is not the installed program")
	}
}

func TestADownloadTakesTheOlderProgramsPlace(t *testing.T) {
	folder := t.TempDir()
	download := filepath.Join(folder, "Downloads", "swcode-desktop.exe")
	_ = os.MkdirAll(filepath.Dir(download), 0o755)
	_ = os.WriteFile(download, []byte("new"), 0o755)
	target := selfinstall.Target(filepath.Join(folder, "Local"))
	if err := selfinstall.Copy(download, target); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(download, []byte("newer"), 0o755)
	if err := selfinstall.Copy(download, target); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(target); string(data) != "newer" {
		t.Fatalf("installed %q", data)
	}
	if entries, _ := os.ReadDir(filepath.Dir(target)); len(entries) != 1 {
		t.Fatalf("left %d files", len(entries))
	}
}
