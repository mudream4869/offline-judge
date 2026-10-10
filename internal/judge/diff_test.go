package judge

import "testing"

func TestSideBySide(t *testing.T) {
	marks := func(ds []DiffLine) string {
		s := ""
		for _, d := range ds {
			switch {
			case !d.HasGot:
				s += "-"
			case !d.HasWant:
				s += "+"
			case d.Same:
				s += "="
			default:
				s += "x"
			}
		}
		return s
	}
	float, _ := ParseCompare("float-diff absolute 1e-3")
	for _, tt := range []struct {
		name      string
		cm        Compare
		got, want string
		marks     string
	}{
		{"line", Compare{}, "1\n2 \n9\n", "1\n2\n3\n", "==x"},
		{"trailing blank lines ignored", Compare{}, "1\n\n\n", "1\n", "="},
		{"got shorter", Compare{}, "1\n", "1\n2\n", "=-"},
		{"got longer", Compare{}, "1\n2\n", "1\n", "=+"},
		{"strict sees spaces and the last newline", Compare{Mode: CompareStrict}, "1 \n2", "1\n2\n", "x=-"},
		{"float tolerance", float, "0.1001\n0.2\n", "0.1\n0.3\n", "=x"},
	} {
		if got := marks(tt.cm.SideBySide(tt.got, tt.want)); got != tt.marks {
			t.Errorf("%s: %s, want %s", tt.name, got, tt.marks)
		}
	}
}
