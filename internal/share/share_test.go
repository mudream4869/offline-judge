package share

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	for _, c := range []Code{
		{Lang: "py", Code: "print(1)\n"},
		{Lang: "cpp", Source: "https://github.com/o/r", Code: "#include <bits/stdc++.h>\nint main() {}\n"},
		{Lang: "go", Code: ""},
		{Lang: "js", Code: "多位元組 ✓\nline2\n\n"},
	} {
		s := Encode(c)
		if url.QueryEscape(s) != s {
			t.Errorf("%q is not URL-safe", s)
		}
		got, err := Decode(s)
		if err != nil || got != c {
			t.Errorf("Decode(Encode(%+v)) = %+v, %v", c, got, err)
		}
	}
}

func TestDecodeRejects(t *testing.T) {
	good := Encode(Code{Lang: "py", Code: "print(1)\n"})
	for name, s := range map[string]string{
		"no version":    good[2:],
		"newer version": "2." + good[2:],
		"not base64":    "1.!!!",
		"cut short":     good[:len(good)/2],
		"empty":         "",
	} {
		if _, err := Decode(s); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// The dictionary pays: real solutions shrink below raw deflate.
func TestDictionaryHelps(t *testing.T) {
	src, err := os.ReadFile("../../problems/0006-rmq/_solutions/ac.cpp")
	if err != nil {
		t.Fatal(err)
	}
	s := Encode(Code{Lang: "cpp", Code: string(src)})
	if len(s) > len(src)*6/10 {
		t.Errorf("%d bytes became %d chars", len(src), len(s))
	}
}

// v1 links must decode forever: pin one.
func TestPinnedLink(t *testing.T) {
	const pinned = "1.Kqjk4krUUUhSsFXAaSBUc6KCtkKSJhdgAA"
	if c, err := Decode(pinned); err != nil || c.Lang != "py" || !strings.Contains(c.Code, "print(a + b)") {
		t.Errorf("pinned link: %+v, %v", c, err)
	}
}
