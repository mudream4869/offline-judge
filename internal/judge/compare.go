package judge

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// CompareMode is how Compare matches outputs.
type CompareMode string

const (
	// CompareLine: lines match after trimming trailing spaces; trailing blank lines are ignored.
	CompareLine CompareMode = "line"
	// CompareStrict: byte for byte.
	CompareStrict CompareMode = "strict"
	// CompareWhite: lines match if their whitespace-separated words do.
	CompareWhite CompareMode = "white-diff"
	// CompareFloat: like CompareWhite, but a number in the answer may be off by Eps.
	CompareFloat CompareMode = "float-diff"
	// CompareTokens: Kattis's default validator. Whitespace-separated words
	// of the whole output match, ignoring case; see the token flags of Compare.
	CompareTokens CompareMode = "tokens"
)

// Compare is a built-in output comparison, the "compare" of problem.json.
// The zero value is CompareLine.
type Compare struct {
	Mode CompareMode
	// For CompareFloat: Eps bounds the absolute error if Abs, the relative
	// error if Rel; either suffices if both.
	Abs, Rel bool
	Eps      float64

	// For CompareTokens, Kattis's flags. A number in the answer may be off
	// by AbsTol if Abs or by RelTol relative if Rel; either suffices if both.
	CaseSensitive  bool // case_sensitive
	SpaceSensitive bool // space_change_sensitive: whitespace must match exactly
	AbsTol, RelTol float64
}

// ParseCompare parses "line", "strict", "white-diff",
// "float-diff [absolute|relative|absolute-relative] [eps]" or
// "tokens [flag...]" with Kattis's validator flags: case_sensitive,
// space_change_sensitive, float_tolerance ε, float_absolute_tolerance ε and
// float_relative_tolerance ε. float-diff defaults to absolute-relative 1e-6.
// "" is line.
func ParseCompare(s string) (Compare, error) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return Compare{Mode: CompareLine}, nil
	}
	cm := Compare{Mode: CompareMode(f[0])}
	switch cm.Mode {
	case CompareLine, CompareStrict, CompareWhite:
		if len(f) > 1 {
			return Compare{}, fmt.Errorf("compare %q 不接受參數", f[0])
		}
		return cm, nil
	case CompareTokens:
		return parseTokenFlags(f[1:])
	case CompareFloat:
	default:
		return Compare{}, fmt.Errorf("不支援的 compare：%q", f[0])
	}

	cm.Abs, cm.Rel, cm.Eps = true, true, 1e-6
	args := f[1:]
	if len(args) > 0 {
		switch args[0] {
		case "absolute":
			cm.Rel = false
			args = args[1:]
		case "relative":
			cm.Abs = false
			args = args[1:]
		case "absolute-relative":
			args = args[1:]
		}
	}
	if len(args) > 0 {
		eps, err := strconv.ParseFloat(args[0], 64)
		if err != nil || !(eps >= 0) || math.IsInf(eps, 0) {
			return Compare{}, fmt.Errorf("float-diff 的誤差有誤：%q", args[0])
		}
		cm.Eps = eps
		args = args[1:]
	}
	if len(args) > 0 {
		return Compare{}, fmt.Errorf("float-diff 的參數有誤：%q", s)
	}
	return cm, nil
}

// parseTokenFlags parses the flags of "tokens".
func parseTokenFlags(args []string) (Compare, error) {
	cm := Compare{Mode: CompareTokens}
	for i := 0; i < len(args); i++ {
		switch flag := args[i]; flag {
		case "case_sensitive":
			cm.CaseSensitive = true
		case "space_change_sensitive":
			cm.SpaceSensitive = true
		case "float_tolerance", "float_absolute_tolerance", "float_relative_tolerance":
			if i+1 == len(args) {
				return Compare{}, fmt.Errorf("tokens 的 %s 缺少誤差", flag)
			}
			i++
			eps, err := strconv.ParseFloat(args[i], 64)
			if err != nil || !(eps >= 0) || math.IsInf(eps, 0) {
				return Compare{}, fmt.Errorf("tokens 的 %s 誤差有誤：%q", flag, args[i])
			}
			if flag != "float_relative_tolerance" {
				cm.Abs, cm.AbsTol = true, eps
			}
			if flag != "float_absolute_tolerance" {
				cm.Rel, cm.RelTol = true, eps
			}
		default:
			return Compare{}, fmt.Errorf("tokens 不支援的參數：%q", flag)
		}
	}
	return cm, nil
}

// String is the form ParseCompare reads.
func (cm Compare) String() string {
	if cm.Mode == CompareTokens {
		f := []string{string(CompareTokens)}
		if cm.CaseSensitive {
			f = append(f, "case_sensitive")
		}
		if cm.SpaceSensitive {
			f = append(f, "space_change_sensitive")
		}
		switch {
		case cm.Abs && cm.Rel && cm.AbsTol == cm.RelTol:
			f = append(f, fmt.Sprintf("float_tolerance %g", cm.AbsTol))
		default:
			if cm.Abs {
				f = append(f, fmt.Sprintf("float_absolute_tolerance %g", cm.AbsTol))
			}
			if cm.Rel {
				f = append(f, fmt.Sprintf("float_relative_tolerance %g", cm.RelTol))
			}
		}
		return strings.Join(f, " ")
	}
	if cm.Mode != CompareFloat {
		return string(cm.mode())
	}
	kind := "absolute-relative"
	if !cm.Rel {
		kind = "absolute"
	} else if !cm.Abs {
		kind = "relative"
	}
	return fmt.Sprintf("%s %s %g", cm.Mode, kind, cm.Eps)
}

// IsDefault reports whether cm is CompareLine, the default.
func (cm Compare) IsDefault() bool { return cm.mode() == CompareLine }

func (cm Compare) mode() CompareMode {
	if cm.Mode == "" {
		return CompareLine
	}
	return cm.Mode
}

// Check reports whether got matches want; msg says where it doesn't.
func (cm Compare) Check(got, want string) (ok bool, msg string) {
	switch cm.mode() {
	case CompareStrict:
		return checkStrict(got, want)
	case CompareWhite, CompareFloat:
		return cm.checkWords(got, want)
	case CompareTokens:
		return cm.checkTokens(got, want)
	}
	return checkLines(got, want)
}

func checkLines(got, want string) (bool, string) {
	g, w := lines(got), lines(want)
	for i := range min(len(g), len(w)) {
		if strings.TrimRight(g[i], " \t\r") != strings.TrimRight(w[i], " \t\r") {
			return false, fmt.Sprintf("第 %d 行不同：預期 %s，得到 %s", i+1, quote(w[i]), quote(g[i]))
		}
	}
	return lineCount(len(g), len(w))
}

func checkStrict(got, want string) (bool, string) {
	if got == want {
		return true, ""
	}
	n := 0 // first differing byte
	for n < len(got) && n < len(want) && got[n] == want[n] {
		n++
	}
	switch {
	case n == len(want):
		return false, fmt.Sprintf("輸出比預期長，多了 %s", quote(got[n:]))
	case n == len(got):
		return false, fmt.Sprintf("輸出比預期短，少了 %s", quote(want[n:]))
	}
	// The prefixes match, so the line starts at the same byte in both.
	start := strings.LastIndexByte(want[:n], '\n') + 1
	lineOf := func(s string) string {
		s = s[start:]
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[:i+1] // keep \n, so a missing \r or space shows
		}
		return s
	}
	return false, fmt.Sprintf("第 %d 行不同：預期 %s，得到 %s",
		strings.Count(want[:n], "\n")+1, quote(lineOf(want)), quote(lineOf(got)))
}

func (cm Compare) checkWords(got, want string) (bool, string) {
	g, w := lines(got), lines(want)
	for i := range min(len(g), len(w)) {
		gw, ww := strings.Fields(g[i]), strings.Fields(w[i])
		for j := range min(len(gw), len(ww)) {
			if cm.wordOK(gw[j], ww[j]) {
				continue
			}
			what := "項"
			if cm.Mode == CompareFloat && isFloat(ww[j]) {
				what = "個數字"
			}
			return false, fmt.Sprintf("第 %d 行第 %d %s不同：預期 %s，得到 %s",
				i+1, j+1, what, quote(ww[j]), quote(gw[j]))
		}
		if len(gw) != len(ww) {
			return false, fmt.Sprintf("第 %d 行預期 %d 項，得到 %d 項", i+1, len(ww), len(gw))
		}
	}
	return lineCount(len(g), len(w))
}

// wordOK compares a word; with CompareFloat a number in want allows an error.
func (cm Compare) wordOK(got, want string) bool {
	if got == want {
		return true
	}
	if cm.Mode != CompareFloat || !isFloat(want) {
		return false
	}
	return near(got, want, cm.Abs, cm.Eps, cm.Rel, cm.Eps)
}

// near reports whether got is a number within absTol (if abs) or relTol
// relative (if rel) of want, a number.
func near(got, want string, abs bool, absTol float64, rel bool, relTol float64) bool {
	g, err := strconv.ParseFloat(got, 64)
	if err != nil {
		return false
	}
	w, _ := strconv.ParseFloat(want, 64)
	var bound float64
	if abs {
		bound = absTol
	}
	if rel {
		bound = max(bound, relTol*math.Abs(w))
	}
	// Slack for decimal rounding, so "0.100001" is within 1e-6 of "0.1".
	return math.Abs(g-w) <= bound*(1+1e-9)
}

// checkTokens compares the whole outputs word by word, like Kattis.
func (cm Compare) checkTokens(got, want string) (bool, string) {
	gw, gs := splitTokens(got)
	ww, ws := splitTokens(want)
	for i := range min(len(gw), len(ww)) {
		g, w := gw[i], ww[i]
		ok := g == w || !cm.CaseSensitive && strings.EqualFold(g, w) ||
			(cm.Abs || cm.Rel) && isFloat(w) && near(g, w, cm.Abs, cm.AbsTol, cm.Rel, cm.RelTol)
		if !ok {
			what := "項"
			if (cm.Abs || cm.Rel) && isFloat(w) {
				what = "個數字"
			}
			return false, fmt.Sprintf("第 %d %s不同：預期 %s，得到 %s", i+1, what, quote(w), quote(g))
		}
		if cm.SpaceSensitive && gs[i] != ws[i] {
			return false, fmt.Sprintf("第 %d 項之前的空白不同：預期 %s，得到 %s", i+1, quote(ws[i]), quote(gs[i]))
		}
	}
	if len(gw) != len(ww) {
		return false, fmt.Sprintf("輸出有 %d 項，預期 %d 項", len(gw), len(ww))
	}
	if cm.SpaceSensitive && gs[len(gs)-1] != ws[len(ws)-1] {
		return false, fmt.Sprintf("結尾的空白不同：預期 %s，得到 %s", quote(ws[len(ws)-1]), quote(gs[len(gs)-1]))
	}
	return true, ""
}

// splitTokens splits s into words and the whitespace before each word,
// plus the trailing whitespace: len(spaces) == len(words)+1.
func splitTokens(s string) (words, spaces []string) {
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v' }
	i := 0
	for {
		j := i
		for j < len(s) && isSpace(s[j]) {
			j++
		}
		spaces = append(spaces, s[i:j])
		if j == len(s) {
			return words, spaces
		}
		k := j
		for k < len(s) && !isSpace(s[k]) {
			k++
		}
		words = append(words, s[j:k])
		i = k
	}
}

// isFloat reports whether an answer word is a real number, not an integer:
// integers still match exactly.
func isFloat(s string) bool {
	if !strings.ContainsAny(s, ".eE") {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// lines splits s into lines, dropping trailing blank ones.
func lines(s string) []string {
	ls := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for len(ls) > 0 && strings.TrimRight(ls[len(ls)-1], " \t\r") == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

func lineCount(got, want int) (bool, string) {
	switch {
	case got < want:
		return false, fmt.Sprintf("輸出只有 %d 行，預期 %d 行", got, want)
	case got > want:
		return false, fmt.Sprintf("輸出有 %d 行，預期只有 %d 行", got, want)
	}
	return true, ""
}

// Longer quoted text is cut to this many runes.
const quoteLimit = 50

// quote shows s in a message, cut if long.
func quote(s string) string {
	if utf8.RuneCountInString(s) > quoteLimit {
		s = string([]rune(s)[:quoteLimit]) + "…"
	}
	return strconv.Quote(s)
}
