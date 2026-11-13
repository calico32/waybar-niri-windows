package module

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"wnw/log"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

// desktopEntry is the subset of a freedesktop .desktop file that we care about.
type desktopEntry struct {
	icon    string
	wmClass string
	name    string
}

// desktopCatalog holds the parsed .desktop entries in the lookup shapes needed
// to match a window's application ID: the desktop file basename and the
// StartupWMClass are used for identification, the display name only as a weaker
// signal.
type desktopCatalog struct {
	byKey      map[string]desktopEntry // basename / StartupWMClass, lower-cased
	byName     map[string]desktopEntry // display name, lower-cased
	byIdentity map[string]desktopEntry // byKey keys without separators
	keys       []string                // byKey keys, longest first
}

var (
	desktopIndexOnce sync.Once
	desktopIndex     desktopCatalog

	pixbufCacheMu sync.Mutex
	pixbufCache   = map[string]*gdk.Pixbuf{}
)

// dataDirs returns the XDG data directories, plus a few locations that are not
// always listed in XDG_DATA_DIRS.
func dataDirs() []string {
	var dirs []string
	seen := map[string]struct{}{}
	add := func(dir string) {
		if dir == "" {
			return
		}
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}

	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		add(dataHome)
	} else if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".local", "share"))
	}

	for _, dataDir := range strings.Split(os.Getenv("XDG_DATA_DIRS"), ":") {
		add(dataDir)
	}
	add("/usr/local/share")
	add("/usr/share")

	return dirs
}

// applicationsDirs returns the directories that may contain .desktop files.
func applicationsDirs() []string {
	dirs := make([]string, 0, len(dataDirs())+3)
	for _, dataDir := range dataDirs() {
		dirs = append(dirs, filepath.Join(dataDir, "applications"))
	}
	dirs = append(dirs,
		"/var/lib/flatpak/exports/share/applications",
		"/var/lib/snapd/desktop/applications",
	)

	return dirs
}

// parseDesktopFile reads the Icon, Name and StartupWMClass fields of a
// .desktop file.
func parseDesktopFile(path string) (desktopEntry, bool) {
	file, err := os.Open(path)
	if err != nil {
		return desktopEntry{}, false
	}
	defer file.Close()

	var entry desktopEntry
	inMainSection := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inMainSection = line == "[Desktop Entry]"
			continue
		}
		if !inMainSection || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Icon":
			entry.icon = value
		case "Name":
			entry.name = value
		case "StartupWMClass":
			entry.wmClass = value
		}
	}

	return entry, entry.icon != ""
}

// loadDesktopEntries indexes the installed .desktop entries.
func loadDesktopEntries() {
	desktopIndex = buildDesktopIndex(applicationsDirs())
	log.Debugf("indexed %d desktop entries for icon lookup", len(desktopIndex.byKey))
}

// buildDesktopIndex indexes the .desktop files found in the given directories.
// Earlier directories win, matching the XDG data directory precedence.
func buildDesktopIndex(dirs []string) desktopCatalog {
	index := desktopCatalog{
		byKey:      map[string]desktopEntry{},
		byName:     map[string]desktopEntry{},
		byIdentity: map[string]desktopEntry{},
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}
			parsed, ok := parseDesktopFile(filepath.Join(dir, entry.Name()))
			if !ok || parsed.icon == "" {
				continue
			}

			keys := []string{strings.TrimSuffix(entry.Name(), ".desktop")}
			if parsed.wmClass != "" {
				keys = append(keys, parsed.wmClass)
			}
			for _, key := range keys {
				key = strings.ToLower(key)
				if _, exists := index.byKey[key]; !exists {
					index.byKey[key] = parsed
				}
				if identity := identityKey(key); identity != "" {
					if _, exists := index.byIdentity[identity]; !exists {
						index.byIdentity[identity] = parsed
					}
				}
			}
			if parsed.name != "" {
				name := strings.ToLower(parsed.name)
				if _, exists := index.byName[name]; !exists {
					index.byName[name] = parsed
				}
			}
		}
	}

	// Longest first, so that prefix matching is deterministic and prefers the
	// most specific key.
	index.keys = make([]string, 0, len(index.byKey))
	for key := range index.byKey {
		index.keys = append(index.keys, key)
	}
	sort.Slice(index.keys, func(a, b int) bool {
		if len(index.keys[a]) != len(index.keys[b]) {
			return len(index.keys[a]) > len(index.keys[b])
		}
		return index.keys[a] < index.keys[b]
	})

	return index
}

// matchPrefix returns the entry whose key is the longest one that the
// application ID extends by a separator, e.g. "google-chrome-Default".
func (i desktopCatalog) matchPrefix(value string) (desktopEntry, bool) {
	for _, key := range i.keys {
		if len(key) < 3 || len(value) <= len(key) || !strings.HasPrefix(value, key) {
			continue
		}
		switch value[len(key)] {
		case '-', '.', '_', '/', ' ':
			return i.byKey[key], true
		}
	}
	return desktopEntry{}, false
}

// identityKey normalises an identifier by dropping the separators and case that
// different toolkits use interchangeably, so that "Startup-WM_Class" and
// "startup.wmclass" compare equal. This mirrors the identity comparison used by
// other Wayland shells (Noctalia, Quickshell).
func identityKey(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if r == '.' || r == '-' || r == '_' || unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// appIdTail drops the reverse-DNS namespace from an application ID, so that
// "org.kde.konsole" is also looked up as "konsole".
func appIdTail(appId string) string {
	tail := appId
	if slash := strings.LastIndex(tail, "/"); slash != -1 && slash+1 < len(tail) {
		tail = tail[slash+1:]
	}
	if dot := strings.LastIndex(tail, "."); dot != -1 && dot+1 < len(tail) {
		tail = tail[dot+1:]
	}
	return tail
}

func desktopEntries() desktopCatalog {
	desktopIndexOnce.Do(loadDesktopEntries)
	return desktopIndex
}

// appIdCandidates returns the possible icon names for an application ID.
func appIdCandidates(appId string) []string {
	seen := map[string]struct{}{}
	var candidates []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, value)
	}

	add(appId)
	add(strings.ToLower(appId))
	add(strings.ReplaceAll(strings.ToLower(appId), " ", "-"))
	add(strings.TrimSuffix(strings.ToLower(appId), ".desktop"))

	// Window IDs opened by a browser (e.g. Chrome PWAs) use IDs such as
	// "chrome-hnpfjngllnobngcgfapefoaidbinmjnm-Default"; also try the leading
	// component.
	if prefix, _, found := strings.Cut(appId, "-"); found && prefix != "" {
		add(strings.ToLower(prefix))
	}

	return candidates
}

// themeHasIcon reports whether the current icon theme provides the named icon.
func themeHasIcon(name string) bool {
	if name == "" {
		return false
	}
	theme, err := gtk.IconThemeGetDefault()
	if err != nil || theme == nil {
		return false
	}
	return theme.HasIcon(name)
}

// themeHasIconFunc is a variable so that tests can stub the icon theme lookup.
var themeHasIconFunc = themeHasIcon

// resolveIconName maps an application ID to an icon name or an absolute icon
// file path.
func resolveIconName(appId string) string {
	appId = strings.TrimSpace(appId)
	if appId == "" {
		return ""
	}

	index := desktopEntries()
	lowered := strings.ToLower(appId)

	// 1. Exact match against a desktop file basename or StartupWMClass.
	if entry, ok := index.byKey[lowered]; ok {
		return entry.icon
	}

	// 2. The same, ignoring case and ".", "-", "_" and spaces.
	if identity := identityKey(lowered); identity != "" {
		if entry, ok := index.byIdentity[identity]; ok {
			return entry.icon
		}
	}

	// 3. The display name, the weakest of the identifying fields.
	if entry, ok := index.byName[lowered]; ok {
		return entry.icon
	}

	// 4. The application ID may name an icon itself. This is tried before the
	//    fuzzy matching below so that an ID such as "vscode-insiders" is not
	//    matched to a different desktop entry that merely shares a word.
	for _, candidate := range appIdCandidates(appId) {
		if themeHasIconFunc(candidate) {
			return candidate
		}
	}
	if path := findIconFile(lowered); path != "" {
		return path
	}

	// 5. Namespaced IDs such as "org.kde.konsole" often run under just the last
	//    component.
	if tail := appIdTail(lowered); tail != lowered {
		if entry, ok := index.byKey[tail]; ok {
			return entry.icon
		}
		if identity := identityKey(tail); identity != "" {
			if entry, ok := index.byIdentity[identity]; ok {
				return entry.icon
			}
		}
	}

	// 6. IDs that carry an extra trailing segment.
	if entry, ok := index.matchPrefix(lowered); ok {
		return entry.icon
	}

	return ""
}

// iconFallbackDirs returns the directories searched for icons that are not part
// of the icon theme: the icon base directories themselves and the XDG pixmap
// directories.
func iconFallbackDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".icons"))
	}
	for _, dataDir := range dataDirs() {
		dirs = append(dirs, filepath.Join(dataDir, "icons"), filepath.Join(dataDir, "pixmaps"))
	}
	return dirs
}

// findIconFile returns the path of a named icon found outside the icon theme,
// following the freedesktop unthemed and pixmap lookup order, or "" if there is
// none.
func findIconFile(name string) string {
	if name == "" {
		return ""
	}
	for _, dir := range iconFallbackDirs() {
		for _, ext := range []string{".svg", ".png"} {
			path := filepath.Join(dir, name+ext)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}

// loadIconPixbuf loads an icon from the current icon theme, from the unthemed
// icon directories, or from disk when the desktop entry points at an absolute
// path.
func loadIconPixbuf(iconName string, size int) *gdk.Pixbuf {
	if iconName == "" || size < 1 {
		return nil
	}

	cacheKey := iconName + "@" + strconv.Itoa(size)
	pixbufCacheMu.Lock()
	cached, ok := pixbufCache[cacheKey]
	pixbufCacheMu.Unlock()
	if ok {
		return cached
	}

	var pixbuf *gdk.Pixbuf
	if filepath.IsAbs(iconName) {
		pixbuf = loadIconFile(iconName, size)
	} else if theme, err := gtk.IconThemeGetDefault(); err == nil && theme != nil {
		loaded, err := theme.LoadIcon(iconName, size, 0)
		if err != nil {
			log.Debugf("failed to load icon %q from the icon theme: %v", iconName, err)
		} else {
			pixbuf = scalePixbufTo(loaded, size)
		}
	}
	if pixbuf == nil {
		// The icon may be a bare name that only exists in an unthemed or pixmap
		// directory, which the icon theme does not cover.
		if path := findIconFile(strings.TrimSuffix(iconName, filepath.Ext(iconName))); path != "" {
			pixbuf = loadIconFile(path, size)
		}
	}
	if pixbuf == nil {
		return nil
	}

	pixbufCacheMu.Lock()
	pixbufCache[cacheKey] = pixbuf
	pixbufCacheMu.Unlock()

	return pixbuf
}

// loadIconFile loads an icon from an absolute path, scaled to size.
func loadIconFile(path string, size int) *gdk.Pixbuf {
	loaded, err := gdk.PixbufNewFromFile(path)
	if err != nil || loaded == nil {
		log.Debugf("failed to load icon file %q: %v", path, err)
		return nil
	}
	return scalePixbufTo(loaded, size)
}

// scalePixbufTo returns the pixbuf scaled to size x size. Icons are square by
// convention, so the aspect ratio is not preserved.
func scalePixbufTo(pixbuf *gdk.Pixbuf, size int) *gdk.Pixbuf {
	if pixbuf == nil || size < 1 {
		return pixbuf
	}
	if pixbuf.GetWidth() == size && pixbuf.GetHeight() == size {
		return pixbuf
	}

	scaled, err := pixbuf.ScaleSimple(size, size, gdk.INTERP_BILINEAR)
	if err != nil || scaled == nil {
		return pixbuf
	}
	return scaled
}

// makeAppIcon renders the application icon as a GtkImage, or returns nil when
// no icon could be resolved.
func (i *Instance) makeAppIcon(appId string, size int) *gtk.Image {
	if size < 1 {
		return nil
	}

	iconName := resolveIconName(appId)
	if iconName == "" {
		return nil
	}

	pixbuf := loadIconPixbuf(iconName, size)
	if pixbuf == nil {
		return nil
	}

	image, err := gtk.ImageNewFromPixbuf(pixbuf)
	if err != nil {
		log.Errorf("error creating icon image: %s", err)
		return nil
	}
	image.SetHAlign(gtk.ALIGN_CENTER)
	image.SetVAlign(gtk.ALIGN_CENTER)

	if style, err := image.GetStyleContext(); err == nil {
		style.AddClass("app-icon")
	}

	return image
}

// appClass converts an application ID into a CSS class name.
func appClass(appId string) string {
	if appId == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range strings.ToLower(appId) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
		if builder.Len() >= 63 {
			break
		}
	}

	return strings.Trim(builder.String(), "-")
}
