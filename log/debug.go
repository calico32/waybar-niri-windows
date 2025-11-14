//go:build debug

package log

// init sets the package global log level to LevelDebug during initialization when built with the debug tag.
func init() {
	global.level = LevelDebug
}