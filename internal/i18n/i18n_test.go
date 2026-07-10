package i18n

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/JeffioZ/idletrigger/internal/version"
)

func TestLocalesHaveMatchingKeys(t *testing.T) {
	en := store["en"]
	zh := store["zh-CN"]
	if len(en) == 0 || len(zh) == 0 {
		t.Fatal("embedded locales were not loaded")
	}
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("zh-CN locale is missing key %q", key)
		}
	}
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("English locale is missing key %q", key)
		}
	}
}

func TestLocalesUseMatchingFormatVerbs(t *testing.T) {
	verbPattern := regexp.MustCompile(`%[-+# 0-9.]*[A-Za-z%]`)
	for key, enText := range store["en"] {
		zhText := store["zh-CN"][key]
		enVerbs := strings.Join(verbPattern.FindAllString(enText, -1), ",")
		zhVerbs := strings.Join(verbPattern.FindAllString(zhText, -1), ",")
		if enVerbs != zhVerbs {
			t.Errorf("format verbs differ for %q: en=%q zh-CN=%q", key, enVerbs, zhVerbs)
		}
	}
}

func TestVersionPlaceholder(t *testing.T) {
	original := version.Value
	version.Value = "v-test"
	t.Cleanup(func() { version.Value = original })

	got := T("en", "version")
	if !strings.Contains(got, "v-test") || strings.Contains(got, "{{version}}") {
		t.Fatalf("version placeholder was not expanded: %q", got)
	}
}

func TestResolveLanguageKeepsExplicitChoice(t *testing.T) {
	if got := ResolveLanguage("en"); got != "en" {
		t.Fatalf("ResolveLanguage(en) = %q", got)
	}
	if got := ResolveLanguage("zh-CN"); got != "zh-CN" {
		t.Fatalf("ResolveLanguage(zh-CN) = %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	d := time.Hour + 2*time.Minute + 3*time.Second
	if got := FormatDuration("en", d); got != "1h 2m 3s" {
		t.Fatalf("English duration = %q", got)
	}
	if got := FormatDuration("zh-CN", d); got != "1 小时 2 分钟 3 秒" {
		t.Fatalf("Chinese duration = %q", got)
	}
}
