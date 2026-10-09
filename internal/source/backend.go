package source

import "context"

// Backend is where the files of a Set come from.
type Backend interface {
	// Latest returns the current version of the source, e.g. a commit sha.
	Latest(ctx context.Context) (string, error)
	// List lists the files at version.
	List(ctx context.Context, version string) ([]File, error)
	// Fetch downloads f at version. The Set checks it against f.SHA.
	Fetch(ctx context.Context, version string, f File) ([]byte, error)
}

// File is a file of a source.
type File struct {
	Path string // relative to the source root
	SHA  string // git blob sha
}

// GitHub is a Backend for a directory in a GitHub repository; versions are commits.
type GitHub struct {
	Repo   Repo
	Client *Client
}

func (g GitHub) Latest(ctx context.Context) (string, error) {
	return g.Client.Commit(ctx, g.Repo)
}

func (g GitHub) List(ctx context.Context, version string) ([]File, error) {
	return g.Client.Tree(ctx, g.Repo, version)
}

func (g GitHub) Fetch(ctx context.Context, version string, f File) ([]byte, error) {
	return g.Client.File(ctx, g.Repo, version, f.Path)
}
