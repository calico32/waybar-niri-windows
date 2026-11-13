package module

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDesktopFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}
}

// swapDesktopIndex replaces the cached desktop index for the duration of a test.
func swapDesktopIndex(t *testing.T, index desktopCatalog) {
	t.Helper()
	previous := desktopIndex
	desktopIndex = index
	desktopIndexOnce.Do(func() {})
	t.Cleanup(func() { desktopIndex = previous })
}

// stubIconTheme replaces the icon theme lookup for the duration of a test, so
// that resolution tests do not depend on the icon theme installed on the
// machine running them.
func stubIconTheme(t *testing.T, has func(string) bool) {
	t.Helper()
	previous := themeHasIconFunc
	themeHasIconFunc = has
	t.Cleanup(func() { themeHasIconFunc = previous })
}

func TestParseDesktopFile(t *testing.T) {
	dir := t.TempDir()
	writeDesktopFile(t, dir, "sample.desktop", `[Desktop Entry]
Type=Application
Name=Sample App
Icon=sample-icon

[Desktop Action New]
Name=New Window
Icon=ignored-icon
`)

	entry, ok := parseDesktopFile(filepath.Join(dir, "sample.desktop"))
	if !ok {
		t.Fatal("expected the desktop file to be parsed")
	}
	if entry.icon != "sample-icon" {
		t.Errorf("expected icon %q, got %q", "sample-icon", entry.icon)
	}
	if entry.name != "Sample App" {
		t.Errorf("expected name %q, got %q", "Sample App", entry.name)
	}
	if entry.wmClass != "" {
		t.Errorf("expected no StartupWMClass, got %q", entry.wmClass)
	}
}

func TestParseDesktopFileWithoutIcon(t *testing.T) {
	dir := t.TempDir()
	writeDesktopFile(t, dir, "no-icon.desktop", "[Desktop Entry]\nName=No Icon\n")

	if _, ok := parseDesktopFile(filepath.Join(dir, "no-icon.desktop")); ok {
		t.Error("expected a desktop file without an Icon field to be rejected")
	}
}

func TestBuildDesktopIndex(t *testing.T) {
	dir := t.TempDir()
	writeDesktopFile(t, dir, "Alacritty.desktop", "[Desktop Entry]\nName=Alacritty\nIcon=Alacritty\nStartupWMClass=Alacritty\n")
	writeDesktopFile(t, dir, "google-chrome.desktop", "[Desktop Entry]\nName=Google Chrome\nIcon=google-chrome\nStartupWMClass=Google-chrome\n")
	writeDesktopFile(t, dir, "com.example.Term.desktop", "[Desktop Entry]\nName=Example Terminal\nIcon=utilities-terminal\nStartupWMClass=example-term\n")
	writeDesktopFile(t, dir, "notes.txt", "not a desktop file")

	index := buildDesktopIndex([]string{dir, filepath.Join(dir, "missing")})

	// A basename and a StartupWMClass that lower-case to the same key collapse
	// into one entry.
	for _, key := range []string{"alacritty", "google-chrome", "com.example.term", "example-term"} {
		if _, ok := index.byKey[key]; !ok {
			t.Errorf("expected index to contain key %q", key)
		}
	}
	if _, ok := index.byName["google chrome"]; !ok {
		t.Error("expected index to contain the display name")
	}
	if _, ok := index.byIdentity["googlechrome"]; !ok {
		t.Error("expected index to contain the separator-free identity key")
	}
	if got := len(index.byKey); got != 4 {
		t.Errorf("expected 4 keys, got %d", got)
	}
}

// Earlier data directories must win, as they do for XDG lookups.
func TestBuildDesktopIndexPrecedence(t *testing.T) {
	user, system := t.TempDir(), t.TempDir()
	writeDesktopFile(t, user, "app.desktop", "[Desktop Entry]\nName=User App\nIcon=user-icon\n")
	writeDesktopFile(t, system, "app.desktop", "[Desktop Entry]\nName=System App\nIcon=system-icon\n")

	index := buildDesktopIndex([]string{user, system})

	if got := index.byKey["app"].icon; got != "user-icon" {
		t.Errorf("expected the first directory to win, got icon %q", got)
	}
}

func TestResolveIconName(t *testing.T) {
	dir := t.TempDir()
	writeDesktopFile(t, dir, "google-chrome.desktop", "[Desktop Entry]\nName=Google Chrome\nIcon=google-chrome\nStartupWMClass=Google-chrome\n")
	writeDesktopFile(t, dir, "org.kde.konsole.desktop", "[Desktop Entry]\nName=Konsole\nIcon=utilities-terminal\n")
	writeDesktopFile(t, dir, "Feishu.desktop", "[Desktop Entry]\nName=Feishu\nIcon=lark\n")

	swapDesktopIndex(t, buildDesktopIndex([]string{dir}))
	stubIconTheme(t, func(string) bool { return false })

	tests := []struct {
		appId string
		want  string
	}{
		{"google-chrome", "google-chrome"},
		{"Google-chrome", "google-chrome"},
		// Case and separator differences are ignored.
		{"Google_Chrome", "google-chrome"},
		// A trailing segment appended by the toolkit.
		{"google-chrome-Default", "google-chrome"},
		{"org.kde.konsole", "utilities-terminal"},
		// Namespaced IDs also resolve through their last component.
		{"konsole", "utilities-terminal"},
		// The display name is used when nothing else matches.
		{"Feishu", "lark"},
	}

	for _, test := range tests {
		if got := resolveIconName(test.appId); got != test.want {
			t.Errorf("resolveIconName(%q) = %q, want %q", test.appId, got, test.want)
		}
	}
}

// An ID that names an icon itself must win over a fuzzy desktop-entry match,
// which is what keeps "vscode-insiders" from resolving to the stable VS Code
// entry that merely shares a word.
func TestResolveIconNamePrefersDirectIconName(t *testing.T) {
	dir := t.TempDir()
	writeDesktopFile(t, dir, "com.microsoft.VSCode.desktop", "[Desktop Entry]\nName=Visual Studio Code\nIcon=vscode\nStartupWMClass=Code\n")

	swapDesktopIndex(t, buildDesktopIndex([]string{dir}))
	stubIconTheme(t, func(name string) bool { return name == "vscode-insiders" })

	if got := resolveIconName("vscode-insiders"); got != "vscode-insiders" {
		t.Errorf("expected the direct icon name to win, got %q", got)
	}
}

func TestResolveIconNameUnknownApp(t *testing.T) {
	swapDesktopIndex(t, desktopCatalog{})
	stubIconTheme(t, func(string) bool { return false })

	if got := resolveIconName(""); got != "" {
		t.Errorf("expected empty icon for an empty app id, got %q", got)
	}
	if got := resolveIconName("  "); got != "" {
		t.Errorf("expected empty icon for a blank app id, got %q", got)
	}
	if got := resolveIconName("no-such-app-anywhere"); got != "" {
		t.Errorf("expected no icon for an unknown app id, got %q", got)
	}
}

func TestFindIconFile(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())

	iconDir := filepath.Join(dataHome, "icons")
	pixmapDir := filepath.Join(dataHome, "pixmaps")
	for _, dir := range []string{iconDir, pixmapDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pixmapIcon := filepath.Join(pixmapDir, "manual-app.png")
	if err := os.WriteFile(pixmapIcon, []byte("not really a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	themeIcon := filepath.Join(iconDir, "themed-app.svg")
	if err := os.WriteFile(themeIcon, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := findIconFile("manual-app"); got != pixmapIcon {
		t.Errorf("findIconFile(%q) = %q, want %q", "manual-app", got, pixmapIcon)
	}
	// Both the icon base directory and the pixmap directory are searched.
	if got := findIconFile("themed-app"); got != themeIcon {
		t.Errorf("findIconFile(%q) = %q, want %q", "themed-app", got, themeIcon)
	}
	if got := findIconFile("missing-app"); got != "" {
		t.Errorf("expected no match for an unknown icon, got %q", got)
	}
	if got := findIconFile(""); got != "" {
		t.Errorf("expected no match for an empty name, got %q", got)
	}
}

func TestAppIdTail(t *testing.T) {
	tests := []struct {
		appId string
		want  string
	}{
		{"org.kde.konsole", "konsole"},
		{"com.example.App/Window", "Window"},
		{"alacritty", "alacritty"},
		// A trailing separator is not a namespace.
		{"org.kde.", "org.kde."},
	}
	for _, test := range tests {
		if got := appIdTail(test.appId); got != test.want {
			t.Errorf("appIdTail(%q) = %q, want %q", test.appId, got, test.want)
		}
	}
}

func TestIdentityKey(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{"Google-chrome", "googlechrome"},
		{"google_chrome", "googlechrome"},
		{"Google Chrome", "googlechrome"},
		{"org.kde.Konsole", "orgkdekonsole"},
	}
	for _, test := range tests {
		if got := identityKey(test.value); got != test.want {
			t.Errorf("identityKey(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestAppClass(t *testing.T) {
	tests := []struct {
		appId string
		want  string
	}{
		{"Alacritty", "alacritty"},
		{"google-chrome", "google-chrome"},
		{"chrome hnpf jng", "chrome-hnpf-jng"},
		{"", ""},
	}
	for _, test := range tests {
		if got := appClass(test.appId); got != test.want {
			t.Errorf("appClass(%q) = %q, want %q", test.appId, got, test.want)
		}
	}
}
