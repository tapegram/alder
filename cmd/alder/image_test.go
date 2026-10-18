package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/ir"
)

// R5: the Render image is the declaration's `image`, else ghcr.io/<owner>/<repo> from the GitHub
// `origin` in <app>/.git/config (a file read), else the E0030 sentence.
func TestImagePath(t *testing.T) {
	gitConfig := func(t *testing.T, body string) string {
		dir := t.TempDir()
		if body != "" {
			os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
			os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(body), 0o644)
		}
		return dir
	}
	origin := func(url string) string {
		return "[core]\n\trepositoryformatversion = 0\n[remote \"upstream\"]\n\turl = https://github.com/other/thing\n[remote \"origin\"]\n\turl = " + url + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n[branch \"main\"]\n\tremote = origin\n"
	}
	const sentence = "E0030 name the registry path: `image: \"ghcr.io/<owner>/<app>\"`"
	for _, c := range []struct {
		name, git, image, want, err string
	}{
		{"declared", "", "ghcr.io/me/mine", "ghcr.io/me/mine", ""},
		{"declared wins over origin", origin("git@github.com:Tavish/Blog.git"), "docker.io/me/mine", "docker.io/me/mine", ""},
		{"scp origin", origin("git@github.com:Tavish/Blog.git"), "", "ghcr.io/tavish/blog", ""},
		{"https origin", origin("https://github.com/t/b"), "", "ghcr.io/t/b", ""},
		{"https origin with .git and slash", origin("https://github.com/Tavish/Blog.git/"), "", "ghcr.io/tavish/blog", ""},
		{"ssh url origin", origin("ssh://git@github.com/Tavish/Blog.git"), "", "ghcr.io/tavish/blog", ""},
		{"gitlab origin", origin("git@gitlab.com:t/b.git"), "", "", sentence},
		{"look-alike host", origin("https://github.com.evil.example/t/b"), "", "", sentence},
		{"no origin", "[core]\n\tbare = false\n[remote \"upstream\"]\n\turl = https://github.com/t/b\n", "", "", sentence},
		{"no git directory", "", "", "", sentence},
		{"a nested path is not a repository", origin("https://github.com/t/b/c"), "", "", sentence},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := imagePath(gitConfig(t, c.git), &ir.Deploy{Image: c.image})
			if c.err != "" {
				if err == nil || !strings.HasPrefix(err.Error(), c.err) {
					t.Fatalf("imagePath = %q, %v; want an error starting %q", got, err, c.err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("imagePath = %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

// The repository is the first `.git` at or above the app: a subdirectory of a repository, a
// worktree's `.git` file (its gitdir's commondir holds the config), and no repository at all.
func TestImagePath_Repository(t *testing.T) {
	const cfg = "[remote \"origin\"]\n\turl = git@github.com:Tavish/Blog.git\n"
	mk := func(p, body string) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("subdirectory", func(t *testing.T) {
		root := t.TempDir()
		mk(filepath.Join(root, ".git", "config"), cfg)
		app := filepath.Join(root, "apps", "blog")
		os.MkdirAll(app, 0o755)
		if got, err := imagePath(app, &ir.Deploy{}); err != nil || got != "ghcr.io/tavish/blog" {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("worktree with commondir", func(t *testing.T) {
		root := t.TempDir()
		mk(filepath.Join(root, "main", ".git", "config"), cfg)
		mk(filepath.Join(root, "main", ".git", "worktrees", "wt", "commondir"), "../..\n")
		mk(filepath.Join(root, "wt", ".git"), "gitdir: "+filepath.Join(root, "main", ".git", "worktrees", "wt")+"\n")
		app := filepath.Join(root, "wt", "app")
		os.MkdirAll(app, 0o755)
		if got, err := imagePath(app, &ir.Deploy{}); err != nil || got != "ghcr.io/tavish/blog" {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("relative gitdir without commondir", func(t *testing.T) {
		root := t.TempDir()
		mk(filepath.Join(root, "gits", "sub", "config"), cfg)
		mk(filepath.Join(root, "sub", ".git"), "gitdir: ../gits/sub\n")
		if got, err := imagePath(filepath.Join(root, "sub"), &ir.Deploy{}); err != nil || got != "ghcr.io/tavish/blog" {
			t.Fatalf("%q %v", got, err)
		}
	})
	t.Run("no repository", func(t *testing.T) {
		app := t.TempDir()
		_, err := imagePath(app, &ir.Deploy{})
		if err == nil || !strings.HasPrefix(err.Error(), "E0030 name the registry path: `image: \"ghcr.io/<owner>/<app>\"`") || !strings.Contains(err.Error(), "no git repository above "+app) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("names the config read", func(t *testing.T) {
		root := t.TempDir()
		mk(filepath.Join(root, ".git", "config"), "[remote \"origin\"]\n\turl = git@gitlab.com:t/b.git\n")
		app := filepath.Join(root, "x")
		os.MkdirAll(app, 0o755)
		_, err := imagePath(app, &ir.Deploy{})
		if err == nil || !strings.Contains(err.Error(), filepath.Join(root, ".git", "config")) {
			t.Fatalf("%v", err)
		}
	})
}
