package main

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"alder/internal/ir"
)

// errNoImage is E0030's sentence for a Render environment with no registry path to push to.
const errNoImage = "E0030 name the registry path: `image: \"ghcr.io/<owner>/<app>\"` (App's `deploy` writes no `image`, and the `origin` remote of %s is not a GitHub repository)"

// imagePath is the registry path the Render environments of d push to (R5): d's `image`, else
// ghcr.io/<owner>/<repo> from the GitHub `origin` remote in <dir>/.git/config (read as a file: no
// git binary), lowercased. With neither it refuses with E0030's sentence. The checker stays free
// of git; the commands that need the path (deploy, status, rollback, logs, restore, ci init) ask
// here.
func imagePath(dir string, d *ir.Deploy) (string, error) {
	if d != nil && d.Image != "" {
		return d.Image, nil
	}
	cfg := filepath.Join(dir, ".git", "config")
	if p, ok := githubImage(originURL(cfg)); ok {
		return p, nil
	}
	return "", errors.New(strings.Replace(errNoImage, "%s", cfg, 1))
}

// originURL is the url of remote "origin" in the git config file at path ("" when there is none).
func originURL(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "["):
			in = strings.Join(strings.Fields(line), " ") == `[remote "origin"]`
		case in:
			if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "url" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// imagePart is what an owner or a repository name is made of in a registry path.
var imagePart = regexp.MustCompile(`^[a-z0-9._-]+$`)

// githubImage is ghcr.io/<owner>/<repo> of a GitHub remote url (`git@github.com:o/r.git`,
// `https://github.com/o/r`, `ssh://git@github.com/o/r.git`); ok is false for any other host or a
// path that is not exactly owner/repo.
func githubImage(url string) (string, bool) {
	var rest string
	switch {
	case strings.HasPrefix(url, "git@github.com:"):
		rest = strings.TrimPrefix(url, "git@github.com:")
	default:
		_, after, ok := strings.Cut(url, "://")
		if !ok {
			return "", false
		}
		host, path, _ := strings.Cut(after, "/")
		if _, h, ok := strings.Cut(host, "@"); ok {
			host = h
		}
		if host != "github.com" {
			return "", false
		}
		rest = path
	}
	rest = strings.TrimSuffix(strings.TrimRight(rest, "/"), ".git")
	owner, repo, ok := strings.Cut(strings.ToLower(rest), "/")
	if !ok || !imagePart.MatchString(owner) || !imagePart.MatchString(repo) {
		return "", false
	}
	return "ghcr.io/" + owner + "/" + repo, true
}
