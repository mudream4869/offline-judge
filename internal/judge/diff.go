package judge

import "strings"

// DiffLine is line N of two outputs side by side. A side without the line
// has Has false.
type DiffLine struct {
	N               int
	Want, Got       string
	HasWant, HasGot bool
	Same            bool // the line matches, by cm
}

// SideBySide pairs the lines of got and want, marking each by whether it
// matches under cm. Lines cm ignores (trailing blank ones) are left out.
func (cm Compare) SideBySide(got, want string) []DiffLine {
	split := lines
	if cm.mode() == CompareStrict {
		split = func(s string) []string { return strings.Split(s, "\n") }
	}
	g, w := split(got), split(want)
	out := make([]DiffLine, max(len(g), len(w)))
	for i := range out {
		d := &out[i]
		d.N = i + 1
		if i < len(w) {
			d.Want, d.HasWant = w[i], true
		}
		if i < len(g) {
			d.Got, d.HasGot = g[i], true
		}
		if d.HasWant && d.HasGot {
			d.Same, _ = cm.Check(d.Got, d.Want)
		}
	}
	return out
}
