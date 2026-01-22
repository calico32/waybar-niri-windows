package jsonc

import (
	"bytes"
	"errors"
	"unicode/utf8"
)

// Sanitize removes all comments from JSONC data.
// It returns an error if the data is not valid UTF-8.
//
// NOTE: it does not checks whether the data is valid JSON or not.
func Sanitize(data []byte) ([]byte, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("jsonc: invalid UTF-8")
	}
	return sanitize(data), nil
}

type state byte

const (
	isString state = 1 << iota
	isCommentLine
	isCommentBlock
	checkNext
)

func sanitize(data []byte) []byte {
	var state state
	return bytes.Map(func(r rune) rune {
		stateCheckNext := state&checkNext != 0
		state &^= checkNext
		switch r {
		case '\n':
			state &^= isCommentLine
		case '\\':
			if state&isString != 0 {
				state |= checkNext
			}
		case '"':
			if state&isString != 0 {
				if stateCheckNext { // escaped quote
					break // switch => write rune
				}
				state &^= isString
			} else if state&(isCommentLine|isCommentBlock) == 0 {
				state |= isString
			}
		case '/':
			if state&isString != 0 {
				break // switch => write rune
			}
			if state&isCommentBlock != 0 {
				if stateCheckNext {
					state &^= isCommentBlock
				} else {
					state |= isCommentLine
				}
			} else {
				if stateCheckNext {
					state |= isCommentLine
				} else {
					state |= checkNext
				}
			}
			return -1 // mark rune for skip
		case '*':
			if state&isString != 0 {
				break // switch => write rune
			}
			if stateCheckNext {
				state |= isCommentBlock
			} else if state&isCommentBlock != 0 {
				state |= checkNext
			}
			return -1 // mark rune for skip
		}
		if state&(isCommentLine|isCommentBlock) != 0 {
			return -1 // mark rune for skip
		}
		return r
	}, data)
}
