package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStore struct {
	root string
}

func NewLocalStore(root string) *LocalStore {
	return &LocalStore{root: root}
}

func (s *LocalStore) Save(relativePath string, reader io.Reader) (string, error) {
	fullPath := filepath.Join(s.root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return "", err
	}

	return fullPath, nil
}

func (s *LocalStore) FullPath(relativePath string) string {
	return filepath.Join(s.root, relativePath)
}

func (s *LocalStore) Delete(relativePath string) error {
	if strings.TrimSpace(relativePath) == "" {
		return nil
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return err
	}
	fullPath, err := filepath.Abs(filepath.Join(root, filepath.Clean(relativePath)))
	if err != nil {
		return err
	}
	relativeToRoot, err := filepath.Rel(root, fullPath)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(filepath.Separator)) {
		return fmt.Errorf("storage path escapes root")
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
