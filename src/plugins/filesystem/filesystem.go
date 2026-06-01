package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathEscapesRoot = errors.New("filesystem: path escapes root")
	ErrFileTooLarge    = errors.New("filesystem: file too large")
)

type Config struct {
	RootDir      string
	MaxReadBytes int64
	AllowWrite   bool
	AllowDelete  bool
	AllowHidden  bool
}

type Service struct {
	root         string
	maxReadBytes int64
	allowWrite   bool
	allowDelete  bool
	allowHidden  bool
}

type Entry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
}

type Stat struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

type SearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func New(cfg Config) (*Service, error) {
	if cfg.RootDir == "" {
		return nil, errors.New("filesystem: root dir is required")
	}

	root, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	maxRead := cfg.MaxReadBytes
	if maxRead <= 0 {
		maxRead = 1 << 20 // 1 MiB default
	}

	return &Service{
		root:         root,
		maxReadBytes: maxRead,
		allowWrite:   cfg.AllowWrite,
		allowDelete:  cfg.AllowDelete,
		allowHidden:  cfg.AllowHidden,
	}, nil
}

func (s *Service) List(ctx context.Context, path string) ([]Entry, error) {
	full, rel, err := s.resolve(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}

	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		name := entry.Name()
		if s.isHidden(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		childRel := filepath.ToSlash(filepath.Join(rel, name))
		if childRel == "." {
			childRel = name
		}

		out = append(out, Entry{
			Path:  childRel,
			Name:  name,
			IsDir: entry.IsDir(),
			Size:  info.Size(),
			Mode:  info.Mode().String(),
		})
	}

	return out, nil
}

func (s *Service) Read(ctx context.Context, path string) (string, error) {
	full, _, err := s.resolve(path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("filesystem: %q is a directory", path)
	}
	if info.Size() > s.maxReadBytes {
		return "", ErrFileTooLarge
	}

	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	return string(data), nil
}

func (s *Service) Write(ctx context.Context, path string, content string) error {
	if !s.allowWrite {
		return errors.New("filesystem: writes are disabled")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	full, _, err := s.resolve(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}

	return os.WriteFile(full, []byte(content), 0o644)
}

func (s *Service) Append(ctx context.Context, path string, content string) error {
	if !s.allowWrite {
		return errors.New("filesystem: writes are disabled")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	full, _, err := s.resolve(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}

func (s *Service) Mkdir(ctx context.Context, path string) error {
	if !s.allowWrite {
		return errors.New("filesystem: writes are disabled")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	full, _, err := s.resolve(path)
	if err != nil {
		return err
	}

	return os.MkdirAll(full, 0o755)
}

func (s *Service) Remove(ctx context.Context, path string) error {
	if !s.allowDelete {
		return errors.New("filesystem: deletes are disabled")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	full, _, err := s.resolve(path)
	if err != nil {
		return err
	}

	if full == s.root {
		return errors.New("filesystem: refusing to delete root")
	}

	return os.RemoveAll(full)
}

func (s *Service) Stat(ctx context.Context, path string) (*Stat, error) {
	full, rel, err := s.resolve(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(full)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return &Stat{
		Path:    filepath.ToSlash(rel),
		Name:    info.Name(),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (s *Service) Search(ctx context.Context, query string, maxResults int) ([]SearchMatch, error) {
	if query == "" {
		return nil, errors.New("filesystem: query is required")
	}
	if maxResults <= 0 {
		maxResults = 50
	}

	var matches []SearchMatch

	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(matches) >= maxResults {
			return filepath.SkipAll
		}

		name := d.Name()
		if s.isHidden(name) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > s.maxReadBytes {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(data), "\n")
		rel, _ := filepath.Rel(s.root, path)

		for i, line := range lines {
			if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
				matches = append(matches, SearchMatch{
					Path: filepath.ToSlash(rel),
					Line: i + 1,
					Text: line,
				})
				if len(matches) >= maxResults {
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return nil, err
	}

	return matches, nil
}

func (s *Service) resolve(path string) (string, string, error) {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		clean = ""
	}

	if filepath.IsAbs(clean) {
		clean = strings.TrimPrefix(clean, string(filepath.Separator))
	}

	full := filepath.Join(s.root, clean)

	abs, err := filepath.Abs(full)
	if err != nil {
		return "", "", err
	}

	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return "", "", err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", ErrPathEscapesRoot
	}

	return abs, rel, nil
}

func (s *Service) isHidden(name string) bool {
	return !s.allowHidden && strings.HasPrefix(name, ".")
}
