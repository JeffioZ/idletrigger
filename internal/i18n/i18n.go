// Package i18n provides multi-language string lookups with automatic
// OS-language detection for zh-CN, falling back to English.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/JeffioZ/idletrigger/internal/version"
)

//go:embed locales/*.json
var localeFS embed.FS

var store = map[string]map[string]string{}

func init() {
	// Load all locale files at startup.
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		panic(fmt.Sprintf("i18n: cannot read locales dir: %v", err))
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			panic(fmt.Sprintf("i18n: cannot read %s: %v", e.Name(), err))
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			panic(fmt.Sprintf("i18n: invalid JSON in %s: %v", e.Name(), err))
		}
		// e.Name() is like "en.json" — strip ".json" for the key.
		lang := e.Name()[:len(e.Name())-5]
		store[lang] = m
	}
}

// detectOSLanguage returns "zh-CN" for Chinese Windows, "en" otherwise.
func detectOSLanguage() string {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("GetUserDefaultUILanguage")
	langID, _, _ := proc.Call()
	// Primary language ID for Chinese is 0x04 (LANG_CHINESE).
	primary := uint16(langID) & 0x3FF
	if primary == 0x04 {
		return "zh-CN"
	}
	return "en"
}

// resolveLanguage maps "auto" to the detected OS language.
func resolveLanguage(userLang string) string {
	if userLang == "auto" {
		return detectOSLanguage()
	}
	return userLang
}

// T returns the translated string for key in the given language (or "auto").
// Falls back to English if the key is missing in either language.
func T(lang string, key string) string {
	l := resolveLanguage(lang)

	m, ok := store[l]
	if !ok {
		m = store["en"]
	}
	if s, ok := m[key]; ok {
		return expand(s)
	}
	// Fallback to English.
	if en, ok := store["en"]; ok {
		if s, ok := en[key]; ok {
			return expand(s)
		}
	}
	return key
}

func expand(s string) string {
	return strings.ReplaceAll(s, "{{version}}", version.Value)
}

// FormatDuration formats a duration for compact user-facing status output.
func FormatDuration(lang string, d time.Duration) string {
	seconds := int64(d.Round(time.Second) / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := seconds % 3600 / 60
	seconds %= 60

	zh := resolveLanguage(lang) == "zh-CN"
	parts := make([]string, 0, 3)
	if hours > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("%d 小时", hours))
		} else {
			parts = append(parts, fmt.Sprintf("%dh", hours))
		}
	}
	if minutes > 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("%d 分钟", minutes))
		} else {
			parts = append(parts, fmt.Sprintf("%dm", minutes))
		}
	}
	if seconds > 0 || len(parts) == 0 {
		if zh {
			parts = append(parts, fmt.Sprintf("%d 秒", seconds))
		} else {
			parts = append(parts, fmt.Sprintf("%ds", seconds))
		}
	}
	return strings.Join(parts, " ")
}
