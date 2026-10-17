package deploy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ArtifactFile is the description an artifact carries.
const ArtifactFile = "alder-deploy.json"

// BinaryName is the executable's name inside an artifact (and a build's directory at the target).
const BinaryName = "app"

// MakeArtifact writes the artifact directory root/<build id>/: the executable (written by
// binary to the path it is given), a copy of the built app's static/ from built (build/<app>/:
// what `alder build` wrote there, the design system's files included) and of the app's
// migrations/ (the files and schema.json, the snapshot the target's plan is made against). The build
// id is the first 16 hex digits of the SHA-256 of the executable: the id the runtime computes
// for itself (rt/web.BuildID) and puts in every page's alder-build meta. An artifact of the same
// id is replaced. alder-deploy.json is written later (WriteDescription), once the plan is known.
func MakeArtifact(root, built, migrations string, binary func(path string) error) (dir, id string, err error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", err
	}
	var rnd [6]byte
	rand.Read(rnd[:])
	tmp := filepath.Join(root, ".new-"+hex.EncodeToString(rnd[:]))
	defer os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", "", err
	}
	exe := filepath.Join(tmp, BinaryName)
	if err := binary(exe); err != nil {
		return "", "", err
	}
	if id, err = fileID(exe); err != nil {
		return "", "", err
	}
	if err := copyTree(filepath.Join(built, "static"), filepath.Join(tmp, "static")); err != nil {
		return "", "", err
	}
	if err := copyTree(migrations, filepath.Join(tmp, "migrations")); err != nil {
		return "", "", err
	}
	dir = filepath.Join(root, id)
	if err := os.RemoveAll(dir); err != nil {
		return "", "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return "", "", err
	}
	return dir, id, nil
}

// WriteDescription writes rec as dir/alder-deploy.json.
func WriteDescription(dir string, rec Record) error {
	return os.WriteFile(filepath.Join(dir, ArtifactFile), rec.JSON(), 0o644)
}

// fileID is the build id of the executable at path.
func fileID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// copyTree copies the directory src to dst (nothing when src does not exist).
func copyTree(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
