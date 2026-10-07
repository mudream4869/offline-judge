// Package source fetches problem sets from GitHub and caches them.
package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Repo is a directory in a GitHub repository.
type Repo struct {
	Owner, Name string
	Ref         string // branch, tag or commit; "HEAD" for the default branch
	Dir         string // "" for the root
}

// Parse parses https://github.com/<owner>/<repo>[/tree/<ref>[/<dir>]].
// A ref can't contain "/".
func Parse(s string) (Repo, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return Repo{}, err
	}
	if u.Host != "github.com" && u.Host != "www.github.com" {
		return Repo{}, fmt.Errorf("不是 GitHub 網址：%s", s)
	}
	seg := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(seg) < 2 {
		return Repo{}, fmt.Errorf("網址缺少 repo：%s", s)
	}
	r := Repo{Owner: seg[0], Name: strings.TrimSuffix(seg[1], ".git"), Ref: "HEAD"}
	switch {
	case len(seg) == 2:
	case seg[2] == "tree" && len(seg) >= 4:
		r.Ref = seg[3]
		r.Dir = strings.Join(seg[4:], "/")
	default:
		return Repo{}, fmt.Errorf("網址要是 repo 或 /tree/<分支>/<資料夾>：%s", s)
	}
	return r, nil
}

// File is a file under Repo.Dir.
type File struct {
	Path string // relative to Repo.Dir
	SHA  string // git blob sha
}

// Client talks to GitHub. Only Commit and Tree count toward the API rate
// limit (60 an hour without a token); files come from raw.githubusercontent.com.
type Client struct {
	HTTP *http.Client
	API  string
	Raw  string
}

// NewClient returns a Client for github.com.
func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 30 * time.Second},
		API:  "https://api.github.com",
		Raw:  "https://raw.githubusercontent.com",
	}
}

// Commit resolves r.Ref to a commit sha.
func (c *Client) Commit(ctx context.Context, r Repo) (string, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/commits/%s", c.API, esc(r.Owner), esc(r.Name), esc(r.Ref))
	bs, err := c.get(ctx, u, "application/vnd.github.sha")
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(bs))
	if len(sha) != 40 {
		return "", fmt.Errorf("GitHub 回傳了奇怪的 commit：%q", sha)
	}
	return sha, nil
}

// Tree lists the files under r.Dir at commit.
func (c *Client) Tree(ctx context.Context, r Repo, commit string) ([]File, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1",
		c.API, esc(r.Owner), esc(r.Name), esc(commit))
	bs, err := c.get(ctx, u, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var t struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(bs, &t); err != nil {
		return nil, err
	}
	if t.Truncated {
		return nil, errors.New("repo 檔案太多，GitHub 無法一次列出")
	}

	prefix := ""
	if r.Dir != "" {
		prefix = r.Dir + "/"
	}
	var out []File
	for _, e := range t.Tree {
		if e.Type == "blob" && strings.HasPrefix(e.Path, prefix) {
			out = append(out, File{Path: e.Path[len(prefix):], SHA: e.SHA})
		}
	}
	return out, nil
}

// File downloads path (relative to r.Dir) at commit.
func (c *Client) File(ctx context.Context, r Repo, commit, path string) ([]byte, error) {
	full := path
	if r.Dir != "" {
		full = r.Dir + "/" + path
	}
	parts := strings.Split(full, "/")
	for i, p := range parts {
		parts[i] = esc(p)
	}
	u := fmt.Sprintf("%s/%s/%s/%s/%s", c.Raw, esc(r.Owner), esc(r.Name), commit,
		strings.Join(parts, "/"))
	return c.get(ctx, u, "")
}

func (c *Client) get(ctx context.Context, u, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("無法連線（%w）", err)
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return bs, nil
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		msg := "GitHub API 已達每小時次數上限"
		if sec, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			msg += "，" + time.Unix(sec, 0).Format("15:04") + " 後再試"
		}
		return nil, errors.New(msg)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("找不到（404）：%s", u)
	}
	return nil, fmt.Errorf("HTTP %d：%s", resp.StatusCode, u)
}

func esc(s string) string { return url.PathEscape(s) }
