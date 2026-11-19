package model

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// The bounds an archive is unpacked within.
const (
	maxEntries  = 4096
	maxUnpacked = 1 << 30
)

var errArchive = errors.New("the archive is not one light unpacks")

// wanted reports whether an archive's file is one the helper loads or a
// licence: the programs an archive of llama.cpp also holds are left in
// it.
func wanted(name string) bool {
	base := path.Base(name)
	return strings.HasSuffix(base, ".dylib") || strings.HasSuffix(base, ".dll") || strings.HasSuffix(base, ".so") ||
		strings.Contains(base, ".so.") || strings.HasPrefix(base, "LICENSE")
}

// local is name as a path under the folder an archive is unpacked in,
// or an error for one that would leave it.
func local(name string) (string, error) {
	clean := filepath.FromSlash(path.Clean(name))
	if strings.Contains(name, `\`) || path.IsAbs(name) || !filepath.IsLocal(clean) {
		return "", fmt.Errorf("%w: it holds %q, which is outside it", errArchive, name)
	}
	return clean, nil
}

// unpacker writes an archive's files under one folder, which nothing it
// writes can leave: every path goes through an os.Root.
type unpacker struct {
	root    *os.Root
	entries int
	written int64
}

func (u *unpacker) count() error {
	if u.entries++; u.entries > maxEntries {
		return fmt.Errorf("%w: it holds more than %d entries", errArchive, maxEntries)
	}
	return nil
}

func (u *unpacker) file(name string, from io.Reader) error {
	if err := u.root.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return err
	}
	file, err := u.root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(file, io.LimitReader(from, maxUnpacked-u.written+1))
	if closed := file.Close(); err == nil {
		err = closed
	}
	if u.written += n; u.written > maxUnpacked {
		return fmt.Errorf("%w: it unpacks to more than %d MB", errArchive, maxUnpacked>>20)
	}
	return err
}

// link makes the link an archive names at name, when it points at a
// file beside it, as a library's short name points at its version.
func (u *unpacker) link(name, target string) error {
	if target != path.Base(target) || target == "." || target == ".." || strings.Contains(target, `\`) {
		return fmt.Errorf("%w: its link %q points outside it", errArchive, name)
	}
	if err := u.root.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return err
	}
	return u.root.Symlink(target, name)
}

// unpack writes the libraries and licences of the archive at source
// (.tar.gz or .zip) under dest, which it makes. An archive with a path
// that leaves dest, a link out of it, or anything that is not a file, a
// folder or a link, is refused whole.
func unpack(source, dest string) error {
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	u := &unpacker{root: root}
	if strings.HasSuffix(strings.TrimSuffix(source, ".part"), ".zip") {
		return u.zip(source)
	}
	return u.tar(source)
}

func (u *unpacker) tar(source string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	packed, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("%w: %w", errArchive, err)
	}
	entries := tar.NewReader(packed)
	for {
		var header *tar.Header
		if header, err = entries.Next(); errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %w", errArchive, err)
		}
		if err = u.count(); err != nil {
			return err
		}
		var name string
		if name, err = local(header.Name); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir, tar.TypeXGlobalHeader, tar.TypeXHeader:
		case tar.TypeReg:
			if wanted(header.Name) {
				err = u.file(name, entries)
			}
		case tar.TypeSymlink:
			if wanted(header.Name) {
				err = u.link(name, header.Linkname)
			}
		default:
			err = fmt.Errorf("%w: %q is neither a file, a folder nor a link", errArchive, header.Name)
		}
		if err != nil {
			return err
		}
	}
}

func (u *unpacker) zip(source string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("%w: %w", errArchive, err)
	}
	defer func() { _ = archive.Close() }()
	for _, entry := range archive.File {
		if err = u.count(); err != nil {
			return err
		}
		var name string
		if name, err = local(entry.Name); err != nil {
			return err
		}
		mode := entry.Mode()
		switch {
		case mode.IsDir():
			continue
		case !mode.IsRegular():
			return fmt.Errorf("%w: %q is neither a file nor a folder", errArchive, entry.Name)
		case !wanted(entry.Name):
			continue
		}
		var from io.ReadCloser
		if from, err = entry.Open(); err != nil {
			return fmt.Errorf("%w: %w", errArchive, err)
		}
		err = u.file(name, from)
		_ = from.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// place finds the folder of the llama.cpp libraries under dir, and the
// one of its ggml backends: official archives and a system's packages
// do not lay them out alike, so they are looked for.
func place(dir string) (placed, error) {
	var p placed
	found := false
	err := filepath.WalkDir(dir, func(at string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, filepath.Dir(at))
		if err != nil {
			return err
		}
		switch name := entry.Name(); {
		case name == "libllama.dylib" || name == "libllama.so" || name == "llama.dll":
			p.Library, found = rel, true
		case isBackend(name):
			p.Backends = rel
		}
		return nil
	})
	if err != nil {
		return placed{}, err
	}
	if !found {
		return placed{}, fmt.Errorf("%w: it holds no llama.cpp library", errArchive)
	}
	if p.Backends == "" {
		p.Backends = p.Library
	}
	return p, nil
}

// isBackend reports whether name is a ggml backend the library loads at
// run time (libggml-metal.dylib, ggml-cpu-haswell.dll…).
func isBackend(name string) bool {
	name = strings.TrimPrefix(name, "lib")
	if !strings.HasPrefix(name, "ggml-") || strings.HasPrefix(name, "ggml-base.") {
		return false
	}
	return strings.HasSuffix(name, ".dylib") || strings.HasSuffix(name, ".so") || strings.HasSuffix(name, ".dll")
}
