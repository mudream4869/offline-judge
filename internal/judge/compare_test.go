package judge

import "testing"

func TestParseCompare(t *testing.T) {
	for s, want := range map[string]string{
		"":                             "line",
		"strict":                       "strict",
		"white-diff":                   "white-diff",
		"float-diff":                   "float-diff absolute-relative 1e-06",
		"float-diff 1e-4":              "float-diff absolute-relative 0.0001",
		"float-diff absolute 0.5":      "float-diff absolute 0.5",
		"float-diff  relative":         "float-diff relative 1e-06",
		"float-diff absolute-relative": "float-diff absolute-relative 1e-06",
	} {
		cm, err := ParseCompare(s)
		if err != nil || cm.String() != want {
			t.Errorf("ParseCompare(%q) = %q, %v; want %q", s, cm, err, want)
		}
	}
	for _, s := range []string{"exact", "line 1", "float-diff x", "float-diff -1",
		"float-diff 1e-6 relative", "float-diff absolute 1 2"} {
		if _, err := ParseCompare(s); err == nil {
			t.Errorf("ParseCompare(%q) accepted", s)
		}
	}
}

func TestCompareCheck(t *testing.T) {
	tests := []struct {
		compare   string
		got, want string
		ok        bool
		msg       string
	}{
		{"", "1\n2  \n\n", "1\n2\n", true, ""},
		{"", "1\n3\n", "1\n2\n", false, `第 2 行不同：預期 "2"，得到 "3"`},
		{"", "1\n", "1\n2\n", false, "輸出只有 1 行，預期 2 行"},
		{"", "1\n2\n3\n", "1\n2\n", false, "輸出有 3 行，預期只有 2 行"},
		{"", "1  2\n", "1 2\n", false, `第 1 行不同：預期 "1 2"，得到 "1  2"`},

		{"strict", "1 2\n", "1 2\n", true, ""},
		{"strict", "1 2 \n", "1 2\n", false, `第 1 行不同：預期 "1 2\n"，得到 "1 2 \n"`},
		{"strict", "a\nb\nc\n", "a\nb\nd\n", false, `第 3 行不同：預期 "d\n"，得到 "c\n"`},
		{"strict", "1\n", "1", false, `輸出比預期長，多了 "\n"`},
		{"strict", "1", "1\n", false, `輸出比預期短，少了 "\n"`},

		{"white-diff", " 1   2\t\n3\r\n\n", "1 2\n3\n", true, ""},
		{"white-diff", "1 2 3\n", "1 2\n3\n", false, "第 1 行預期 2 項，得到 3 項"},
		{"white-diff", "1 x\n", "1 2\n", false, `第 1 行第 2 項不同：預期 "2"，得到 "x"`},
		{"white-diff", "1.0\n", "1\n", false, `第 1 行第 1 項不同：預期 "1"，得到 "1.0"`},

		{"float-diff", "0.100001 2\n", "0.1 2\n", true, ""},
		{"float-diff", "1000001.5\n", "1000000.5\n", true, ""}, // relative
		{"float-diff", "1e-7\n", "0.0\n", true, ""},            // absolute
		{"float-diff", "0.10001\n", "0.1\n", false,
			`第 1 行第 1 個數字不同：預期 "0.1"，得到 "0.10001"`},
		{"float-diff", "3\n", "2\n", false, `第 1 行第 1 項不同：預期 "2"，得到 "3"`}, // integers are exact
		{"float-diff", "x\n", "0.5\n", false, `第 1 行第 1 個數字不同：預期 "0.5"，得到 "x"`},
		{"float-diff", "nan\n", "0.5\n", false, `第 1 行第 1 個數字不同：預期 "0.5"，得到 "nan"`},
		{"float-diff absolute 1e-6", "1000001.5\n", "1000000.5\n", false,
			`第 1 行第 1 個數字不同：預期 "1000000.5"，得到 "1000001.5"`},
		{"float-diff relative 1e-6", "1e-7\n", "0.0\n", false,
			`第 1 行第 1 個數字不同：預期 "0.0"，得到 "1e-7"`},
	}
	for _, tt := range tests {
		cm, err := ParseCompare(tt.compare)
		if err != nil {
			t.Fatal(err)
		}
		ok, msg := cm.Check(tt.got, tt.want)
		if ok != tt.ok || msg != tt.msg {
			t.Errorf("%q.Check(%q, %q) = %v, %q; want %v, %q",
				tt.compare, tt.got, tt.want, ok, msg, tt.ok, tt.msg)
		}
	}
}

func TestQuoteCuts(t *testing.T) {
	long := make([]rune, quoteLimit+10)
	for i := range long {
		long[i] = '字'
	}
	if got, want := quote(string(long)), `"`+string(long[:quoteLimit])+`…"`; got != want {
		t.Errorf("quote = %q, want %q", got, want)
	}
}
