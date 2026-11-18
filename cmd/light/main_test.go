package main

import (
	"path/filepath"
	"testing"
)

func TestTheHistoryFolderIsTheOneGivenAndNoneForAHiddenRun(t *testing.T) {
	t.Setenv("LIGHT_DIR", "/somewhere")
	t.Setenv("EMBER_HIDDEN", "1")
	if got := historyDir(); got != "/somewhere" {
		t.Fatalf("LIGHT_DIR gives %q", got)
	}
	t.Setenv("LIGHT_DIR", "")
	if got := historyDir(); got != "" {
		t.Fatalf("a hidden run keeps its history in %q", got)
	}
	t.Setenv("EMBER_HIDDEN", "")
	if got := historyDir(); filepath.Base(got) != "light" {
		t.Fatalf("a shown run keeps its history in %q", got)
	}
}

func TestTheEnvironmentOnlyOverridesWhatTheSettingsSay(t *testing.T) {
	t.Setenv("EMBER_HIDDEN", "")
	t.Setenv("LIGHT_DIR", "/kept")
	t.Setenv("LIGHT_DOWNLOADS", "/elsewhere")
	t.Setenv("HOME", "/home/someone")
	usual := filepath.Join("/home/someone", "Downloads")
	if got := options(); got.Dir != "/kept" || got.Downloads != "/elsewhere" || got.DefaultDownloads != usual ||
		got.Files == nil {
		t.Fatalf("the options are %+v", got)
	}
	t.Setenv("LIGHT_DOWNLOADS", "")
	if got := options(); got.Downloads != "" || got.DefaultDownloads != usual {
		t.Fatalf("with no folder given, the settings' is overridden by %q, the system's is %q", got.Downloads,
			got.DefaultDownloads)
	}
}
