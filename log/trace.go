//go:build trace

package log

func init() {
	global.level = LevelTrace
}
