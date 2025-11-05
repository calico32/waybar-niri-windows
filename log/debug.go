//go:build debug

package log

func init() {
	global.level = LevelDebug
}
