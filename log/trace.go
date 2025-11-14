//go:build trace

package log

// init sets the package's global logging level to LevelTrace when the package is built with the "trace" build tag.
func init() {
	global.level = LevelTrace
}