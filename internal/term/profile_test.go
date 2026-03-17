package term

import "testing"

func TestDetectProfilePrefersExplicitEncoding(t *testing.T) {
	p := DetectProfile("xterm-256color", "en_US.UTF-8", "cp437", 80, 25, true)
	if p.Encoding != EncodingCP437 {
		t.Fatalf("expected cp437, got %s", p.Encoding)
	}
}

func TestDetectProfileFallsBackToLocaleUTF8(t *testing.T) {
	p := DetectProfile("xterm", "en_US.UTF-8", "", 80, 25, true)
	if p.Encoding != EncodingUTF8 {
		t.Fatalf("expected utf8, got %s", p.Encoding)
	}
}

func TestDetectProfileSyncTermDefaultsToCP437WithoutUTF8Locale(t *testing.T) {
	p := DetectProfile("syncterm", "C", "", 80, 25, true)
	if p.Encoding != EncodingCP437 {
		t.Fatalf("expected cp437 for syncterm non-utf8 locale, got %s", p.Encoding)
	}
	if !p.SyncTERM {
		t.Fatal("expected syncterm flag")
	}
}

func TestDetectProfileForcesASCIIWhenANSIOff(t *testing.T) {
	p := DetectProfile("xterm", "en_US.UTF-8", "", 80, 25, false)
	if p.Encoding != EncodingASCII {
		t.Fatalf("expected ascii when ansi disabled, got %s", p.Encoding)
	}
	if SupportsUnicode(p) {
		t.Fatal("unicode should not be supported for ansi-off profile")
	}
}
