package module

import (
	"encoding/json"
	"fmt"
	"regexp"
	"wnw/niri"
)

type Config struct {
	Mode        Mode         `json:"mode"`
	Symbols     niri.Symbols `json:"symbols"`
	WindowRules WindowRules  `json:"rules"`
}

type Mode string

const (
	TextMode      Mode = "text"
	GraphicalMode Mode = "graphical"
)

func (m *Mode) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	switch s {
	case "text":
		*m = TextMode
	case "graphical":
		*m = GraphicalMode
	default:
		return fmt.Errorf("unknown mode: %s", s)
	}
	return nil
}

type WindowRuleConfig struct {
	AppId    string `json:"app-id"`
	Title    string `json:"title"`
	Class    string `json:"class"`
	Continue bool   `json:"continue"`
}

type WindowRule struct {
	AppId    *regexp.Regexp
	Title    *regexp.Regexp
	Class    string
	Continue bool
}

type WindowRules []WindowRule

func (w *WindowRules) UnmarshalJSON(data []byte) error {
	var rules []WindowRuleConfig
	err := json.Unmarshal(data, &rules)
	if err != nil {
		return fmt.Errorf("error unmarshaling rules: %w", err)
	}
	s := make([]WindowRule, len(rules))
	for idx, rule := range rules {
		if rule.AppId != "" {
			s[idx].AppId, err = regexp.Compile(rule.AppId)
			if err != nil {
				return fmt.Errorf("error compiling regex: %w", err)
			}
		}
		if rule.Title != "" {
			s[idx].Title, err = regexp.Compile(rule.Title)
			if err != nil {
				return fmt.Errorf("error compiling regex: %w", err)
			}
		}
		s[idx].Class = rule.Class
		s[idx].Continue = rule.Continue
	}
	*w = s
	return nil
}
