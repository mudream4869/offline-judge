//go:build js && wasm

package main

import (
	"sync"

	"github.com/voilelab/toolgui/toolgui/tgwasm"

	"github.com/mudream4869/offline-judge/internal/judge"
)

type runner interface {
	judge.Runner
	Ready() bool
	// awaitLoad blocks until the first load succeeds or fails.
	awaitLoad()
}

type lang struct {
	name    string
	id      string
	hl      string // code highlight language
	code    string // default code
	gen     string // default stress-test generator: reads a seed, prints an input
	loading string // shown while the runtime loads
	// interactive: the runner can talk to an interactor (interactor.js).
	interactive bool
	newRun      func() runner
	once        sync.Once
	run         runner
}

// checkRunner starts the checker worker on first use.
var checkRunner = sync.OnceValue(func() *CheckRunner {
	return NewCheckRunner(assetURL("checker.mjs"))
})

// runner starts the runtime on first use, so unused ones aren't downloaded.
// The problem page reruns once it loads, to update the sidebar status.
func (l *lang) runner() runner {
	l.once.Do(func() {
		l.run = l.newRun()
		go func() {
			l.run.awaitLoad()
			app.RerunPage("problems")
		}()
	})
	return l.run
}

var (
	langs = []*lang{
		{
			name:        "Python",
			id:          "py",
			gen:         "import random\n\nrandom.seed(int(input()))\nn = random.randint(1, 10)\nprint(n)\n",
			hl:          "python",
			code:        "import sys\ninput = sys.stdin.readline\n\n",
			loading:     "載入中（首次約需數秒）",
			interactive: true,
			newRun:      func() runner { return NewCodeRunner(assetURL("pyworker.mjs"), "Python") },
		},
		{
			name: "C++",
			id:   "cpp",
			gen: "#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n" +
				"    unsigned seed;\n    cin >> seed;\n    mt19937 rng(seed);\n" +
				"    int n = rng() % 10 + 1;\n    cout << n << \"\\n\";\n}\n",
			hl: "cpp",
			code: "#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n" +
				"    ios::sync_with_stdio(false);\n    cin.tie(nullptr);\n\n    return 0;\n}\n",
			loading:     "載入中（首次需下載約 27 MB 的 clang）",
			interactive: true,
			newRun: func() runner {
				return NewCompileRunner("C++", assetURL("cppcompile.mjs"), assetURL("wasirun.mjs"))
			},
		},
		{
			name: "JavaScript",
			id:   "js",
			gen: "let s = Number(require('fs').readFileSync(0, 'utf8')) >>> 0\n" +
				"// mulberry32\nconst rand = () => {\n  s = (s + 0x6d2b79f5) >>> 0\n  let t = s\n" +
				"  t = Math.imul(t ^ (t >>> 15), t | 1)\n  t ^= t + Math.imul(t ^ (t >>> 7), t | 61)\n" +
				"  return ((t ^ (t >>> 14)) >>> 0) / 4294967296\n}\n" +
				"const randint = (lo, hi) => lo + Math.floor(rand() * (hi - lo + 1))\n\n" +
				"console.log(randint(1, 10))\n",
			hl:          "javascript",
			code:        "const lines = require('fs').readFileSync(0, 'utf8').split('\\n')\n\n",
			loading:     "載入中",
			interactive: true,
			newRun:      func() runner { return NewCodeRunner(assetURL("jsrun.mjs"), "JavaScript") },
		},
		{
			name: "Go",
			id:   "go",
			gen: "package main\n\nimport (\n\t\"fmt\"\n\t\"math/rand\"\n)\n\nfunc main() {\n" +
				"\tvar seed int64\n\tfmt.Scan(&seed)\n\tr := rand.New(rand.NewSource(seed))\n" +
				"\tn := r.Intn(10) + 1\n\tfmt.Println(n)\n}\n",
			hl: "go",
			code: "package main\n\nimport (\n\t\"bufio\"\n\t\"fmt\"\n\t\"os\"\n)\n\nfunc main() {\n" +
				"\tin := bufio.NewReader(os.Stdin)\n\tout := bufio.NewWriter(os.Stdout)\n" +
				"\tdefer out.Flush()\n\n\tvar n int\n\tfmt.Fscan(in, &n)\n}\n",
			loading:     "載入中（首次需下載約 17 MB 的 Go 編譯器）",
			interactive: true,
			newRun: func() runner {
				return NewCompileRunner("Go", assetURL("gocompile.mjs"), assetURL("wasirun.mjs"))
			},
		},
	}
)

func langByID(id string) *lang {
	for _, l := range langs {
		if l.id == id {
			return l
		}
	}
	return &lang{name: id, id: id}
}

// assetURL returns the URL of a web/ file; names are constants, so an error is a bug.
func assetURL(name string) string {
	u, err := tgwasm.AssetURL(name)
	if err != nil {
		panic(err)
	}
	return u
}

// userTemplate is lg's default code as set in 設定, or lg.code.
func userTemplate(lg *lang) string {
	if t := memo.getText(templateKey(lg), ""); t != "" {
		return t
	}
	return lg.code
}

// setUserTemplate saves code as lg's default; lg.code is saved as "", so
// it follows changes to the built-in one.
func setUserTemplate(lg *lang, code string) {
	if old := userTemplate(lg); old != code {
		memo.forgetDefaults("code_"+lg.id+"_", old)
	}
	if code == lg.code {
		code = ""
	}
	memo.setText(templateKey(lg), code)
}

func templateKey(lg *lang) string { return "template_" + lg.id }
