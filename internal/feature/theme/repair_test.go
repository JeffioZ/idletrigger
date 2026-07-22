package theme

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPatchThemeFileForcesColorizationAndCurrentModes(t *testing.T) {
	source := []byte("[Theme]\r\nDisplayName=Original\r\nThemeId={OLD}\r\n\r\n[VisualStyles]\r\nAutoColorization=1\r\nColorizationColor=0X112233\r\nAppMode=Light\r\nSystemMode=Light\r\n\r\n[Sounds]\r\nSchemeName=Default\r\n")
	got, err := patchThemeFile(source, themeFilePatch{
		displayName: "IdleTrigger DWM Refresh", themeID: "{NEW}",
		appMode: "Dark", systemMode: "Dark", colorization: 0xff123456,
		setColorization: true, disableAutoColorization: true,
	})
	if err != nil {
		t.Fatalf("patchThemeFile: %v", err)
	}
	text := string(got)
	for _, want := range []string{
		"DisplayName=IdleTrigger DWM Refresh", "ThemeId={NEW}",
		"AutoColorization=0", "ColorizationColor=0XFF123456",
		"AppMode=Dark", "SystemMode=Dark",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("patched theme missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "[Sounds]\r\nSchemeName=Default") {
		t.Fatalf("unrelated theme section changed:\n%s", text)
	}
}

func TestPatchRestoreThemeUsesCurrentColorizationAndPreservesAutoMode(t *testing.T) {
	source := []byte("[Theme]\nDisplayName=Original\nThemeId={OLD}\n[VisualStyles]\nAutoColorization=1\nColorizationColor=0XABCDEF\n")
	got, err := patchThemeFile(source, themeFilePatch{
		displayName: "IdleTrigger DWM Restore", themeID: "{NEW}",
		appMode: "Dark", systemMode: "Light", colorization: 0xff123456, setColorization: true,
	})
	if err != nil {
		t.Fatalf("patchThemeFile: %v", err)
	}
	text := string(got)
	for _, want := range []string{"AutoColorization=1", "ColorizationColor=0XFF123456", "AppMode=Dark", "SystemMode=Light"} {
		if !strings.Contains(text, want) {
			t.Errorf("restore theme missing %q:\n%s", want, text)
		}
	}
}

func TestPatchThemeFileRejectsMissingRequiredSection(t *testing.T) {
	_, err := patchThemeFile([]byte("[Theme]\nDisplayName=Original\n"), themeFilePatch{
		displayName: "Refresh", themeID: "{NEW}", appMode: "Dark", systemMode: "Dark",
	})
	if err == nil || !strings.Contains(err.Error(), "VisualStyles") {
		t.Fatalf("patchThemeFile error = %v, want missing VisualStyles", err)
	}
}

func TestNudgedColorizationChangesOnlyLowNibble(t *testing.T) {
	for _, test := range []struct {
		input uint32
		want  uint32
	}{
		{0xff123456, 0xff123457},
		{0xff123459, 0xff123458},
		{0xff12345f, 0xff12345e},
	} {
		if got := nudgedColorization(test.input); got != test.want {
			t.Errorf("nudgedColorization(%08X) = %08X, want %08X", test.input, got, test.want)
		}
	}
}

func TestThemeIDStringHasSingleBracePair(t *testing.T) {
	id := windows.GUID{Data1: 0x12345678, Data2: 0x1234, Data3: 0xabcd, Data4: [8]byte{0x80, 0, 1, 2, 3, 4, 5, 6}}
	got := themeIDString(id)
	if strings.Count(got, "{") != 1 || strings.Count(got, "}") != 1 || got != strings.ToUpper(got) {
		t.Fatalf("themeIDString = %q, want one uppercase brace pair", got)
	}
}
