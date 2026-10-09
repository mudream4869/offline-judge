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
}

type lang struct {
	name    string
	id      string
	hl      string // code highlight language
	code    string // default code
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
func (l *lang) runner() runner {
	l.once.Do(func() { l.run = l.newRun() })
	return l.run
}

var (
	langs = []*lang{
		{
			name:        "Python",
			id:          "py",
			hl:          "python",
			code:        "import sys\ninput = sys.stdin.readline\n\n",
			loading:     "載入中（首次約需數秒）",
			interactive: true,
			newRun:      func() runner { return NewCodeRunner(assetURL("pyworker.mjs"), "Python") },
		},
		{
			name: "C++",
			id:   "cpp",
			hl:   "cpp",
			code: "#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n" +
				"    ios::sync_with_stdio(false);\n    cin.tie(nullptr);\n\n    return 0;\n}\n",
			loading:     "載入中（首次需下載約 27 MB 的 clang）",
			interactive: true,
			newRun: func() runner {
				return NewCompileRunner("C++", assetURL("cppcompile.mjs"), assetURL("wasirun.mjs"))
			},
		},
		{
			name:    "JavaScript",
			id:      "js",
			hl:      "javascript",
			code:    "const lines = require('fs').readFileSync(0, 'utf8').split('\\n')\n\n",
			loading: "載入中",
			newRun:  func() runner { return NewCodeRunner(assetURL("jsrun.mjs"), "JavaScript") },
		},
		{
			name: "Go",
			id:   "go",
			hl:   "go",
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
