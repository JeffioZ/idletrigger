package popup

import "testing"

func TestTimeoutChoices(t *testing.T) {
	choices, selected := timeoutChoices(30, true)
	if len(choices) != 15 || choices[selected].minutes != 30 || choices[selected].label != "30 分钟" {
		t.Fatalf("unexpected preset choices: %#v, selected=%d", choices, selected)
	}

	choices, selected = timeoutChoices(90, false)
	if len(choices) != 16 || choices[selected].minutes != 90 || choices[selected].label != "90 minutes" {
		t.Fatalf("custom timeout was not preserved: %#v, selected=%d", choices, selected)
	}
}

func TestFormatTimeout(t *testing.T) {
	if got := formatTimeout(60, false); got != "1 hour" {
		t.Fatalf("formatTimeout(60) = %q", got)
	}
	if got := formatTimeout(120, true); got != "2 小时" {
		t.Fatalf("formatTimeout(120) = %q", got)
	}
}
