// Package share packs a program into a short, URL-safe string, so code can
// be shared by link: raw deflate with a preset dictionary of what programs
// often start with, then base64url.
package share

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// Code is a shared program.
type Code struct {
	Lang   string // language id: py, cpp, js, go
	Source string // problem source URL; "" for the default source
	Code   string
}

// v1 is the version prefix of Encode's output. A link must decode forever,
// so the dictionary of a version never changes; a new one gets a new prefix.
const v1 = "1."

// MaxCode caps the decoded code, so a crafted link can't inflate without end.
const MaxCode = 1 << 20

// dict1 is v1's dictionary. Deflate matches cost less the nearer they are,
// so the most common text is last.
var dict1 = []byte(`package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	var n int
	fmt.Fscan(in, &n)
	for i := 0; i < n; i++ {
	fmt.Fprintln(out, 
}
const lines = require('fs').readFileSync(0, 'utf8').split('\n')
const rl = require('readline').createInterface({ input: process.stdin })
rl.on('line', (line) => {
console.log(
for (let i = 0; i < n; i++) {
module.exports = { 
function 
#include <bits/stdc++.h>
using namespace std;

typedef long long ll;
int main() {
    ios::sync_with_stdio(false);
    cin.tie(nullptr);

    int n;
    cin >> n;
    vector<int> a(n);
    for (int i = 0; i < n; i++) {
        cin >> a[i];
    }
    cout << ans << "\n";
    return 0;
}
long long 
import sys
input = sys.stdin.readline

n = int(input())
a = list(map(int, input().split()))
for i in range(n):
print(
def 
    return 
`)

// Encode packs c; Decode unpacks it.
func Encode(c Code) string {
	var buf bytes.Buffer
	w, _ := flate.NewWriterDict(&buf, flate.BestCompression, dict1)
	io.WriteString(w, c.Lang+"\n"+c.Source+"\n"+c.Code)
	w.Close()
	return v1 + base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

var errBad = errors.New("分享的程式碼不完整或已損壞")

// Decode unpacks what Encode made.
func Decode(s string) (Code, error) {
	data, ok := strings.CutPrefix(s, v1)
	if !ok {
		return Code{}, errors.New("不認得的分享連結版本，請更新網頁")
	}
	raw, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return Code{}, errBad
	}
	r := flate.NewReaderDict(bytes.NewReader(raw), dict1)
	text, err := io.ReadAll(io.LimitReader(r, MaxCode+1))
	if err != nil {
		return Code{}, errBad
	}
	if len(text) > MaxCode {
		return Code{}, errors.New("分享的程式碼太長")
	}
	lang, rest, ok1 := strings.Cut(string(text), "\n")
	src, code, ok2 := strings.Cut(rest, "\n")
	if !ok1 || !ok2 || lang == "" {
		return Code{}, errBad
	}
	return Code{Lang: lang, Source: src, Code: code}, nil
}
