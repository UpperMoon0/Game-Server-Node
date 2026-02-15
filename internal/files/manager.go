package files

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager handles file operations for the node agent
type Manager struct {
	basePath string
}

// NewManager creates a new file manager
func NewManager(basePath string) *Manager {
	return &Manager{
		basePath: basePath,
	}
}

// FileInfo represents information about a file
type FileInfo struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	IsDirectory  bool   `json:"is_directory"`
	Size         int64  `json:"size"`
	ModifiedTime int64  `json:"modified_time"`
	CreatedTime  int64  `json:"created_time"`
	Permissions  string `json:"permissions"`
}

// ListFiles lists files in a directory
func (m *Manager) ListFiles(path string, recursive bool) ([]FileInfo, error) {
	fullPath := m.resolvePath(path)

	// Check if path exists
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to access path: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}

	var files []FileInfo

	if recursive {
		err = filepath.Walk(fullPath, func(walkPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(m.basePath, walkPath)
			if err != nil {
				return err
			}

			files = append(files, FileInfo{
				Name:         info.Name(),
				Path:         relPath,
				IsDirectory:  info.IsDir(),
				Size:         info.Size(),
				ModifiedTime: info.ModTime().Unix(),
				CreatedTime:  info.ModTime().Unix(), // Go doesn't have created time cross-platform
				Permissions:  info.Mode().String(),
			})

			return nil
		})
	} else {
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range entries {
			fullEntryPath := filepath.Join(fullPath, entry.Name())
			info, err := entry.Info()
			if err != nil {
				continue
			}

			relPath, err := filepath.Rel(m.basePath, fullEntryPath)
			if err != nil {
				continue
			}

			files = append(files, FileInfo{
				Name:         entry.Name(),
				Path:         relPath,
				IsDirectory:  entry.IsDir(),
				Size:         info.Size(),
				ModifiedTime: info.ModTime().Unix(),
				CreatedTime:  info.ModTime().Unix(),
				Permissions:  info.Mode().String(),
			})
		}
	}

	return files, err
}

// CreateFile creates a new file or directory
func (m *Manager) CreateFile(path string, isDir bool) error {
	fullPath := m.resolvePath(path)

	if isDir {
		return os.MkdirAll(fullPath, 0755)
	}

	// Create parent directories if needed
	parentDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	file.Close()

	return nil
}

// DeleteFile deletes a file or directory
func (m *Manager) DeleteFile(path string, recursive bool) error {
	fullPath := m.resolvePath(path)

	info, err := os.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("failed to access path: %w", err)
	}

	if info.IsDir() {
		if recursive {
			return os.RemoveAll(fullPath)
		}
		return os.Remove(fullPath)
	}

	return os.Remove(fullPath)
}

// RenameFile renames a file or directory
func (m *Manager) RenameFile(oldPath, newPath string) error {
	fullOldPath := m.resolvePath(oldPath)
	fullNewPath := m.resolvePath(newPath)

	// Create parent directories if needed
	parentDir := filepath.Dir(fullNewPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	return os.Rename(fullOldPath, fullNewPath)
}

// MoveFile moves a file or directory
func (m *Manager) MoveFile(sourcePath, destPath string) error {
	return m.RenameFile(sourcePath, destPath)
}

// CopyFile copies a file or directory
func (m *Manager) CopyFile(sourcePath, destPath string, recursive bool) error {
	fullSourcePath := m.resolvePath(sourcePath)
	fullDestPath := m.resolvePath(destPath)

	info, err := os.Stat(fullSourcePath)
	if err != nil {
		return fmt.Errorf("failed to access source: %w", err)
	}

	if info.IsDir() {
		if !recursive {
			return fmt.Errorf("cannot copy directory without recursive flag")
		}
		return m.copyDirectory(fullSourcePath, fullDestPath)
	}

	return m.copySingleFile(fullSourcePath, fullDestPath)
}

// copySingleFile copies a single file
func (m *Manager) copySingleFile(source, dest string) error {
	// Create parent directories if needed
	parentDir := filepath.Dir(dest)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("failed to create dest file: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// Copy permissions
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	return os.Chmod(dest, sourceInfo.Mode())
}

// copyDirectory recursively copies a directory
func (m *Manager) copyDirectory(source, dest string) error {
	// Create dest directory
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("failed to create dest directory: %w", err)
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := m.copyDirectory(sourcePath, destPath); err != nil {
				return err
			}
		} else {
			if err := m.copySingleFile(sourcePath, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// ReadFile reads file content
func (m *Manager) ReadFile(path string, offset, length int64) ([]byte, int64, error) {
	fullPath := m.resolvePath(path)

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to access file: %w", err)
	}

	if info.IsDir() {
		return nil, 0, fmt.Errorf("path is a directory")
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	totalSize := info.Size()

	// If offset is specified, seek to that position
	if offset > 0 {
		if _, err := file.Seek(offset, 0); err != nil {
			return nil, 0, fmt.Errorf("failed to seek: %w", err)
		}
	}

	// If length is specified, read only that amount
	if length > 0 {
		buf := make([]byte, length)
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return nil, 0, fmt.Errorf("failed to read file: %w", err)
		}
		return buf[:n], totalSize, nil
	}

	// Read entire file
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read file: %w", err)
	}

	return content, totalSize, nil
}

// WriteFile writes content to a file
func (m *Manager) WriteFile(path string, content []byte, append bool) error {
	fullPath := m.resolvePath(path)

	// Create parent directories if needed
	parentDir := filepath.Dir(fullPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	var file *os.File
	var err error

	if append {
		file, err = os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	} else {
		file, err = os.Create(fullPath)
	}

	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(content)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// FileExists checks if a file exists
func (m *Manager) FileExists(path string) (bool, bool, error) {
	fullPath := m.resolvePath(path)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("failed to access path: %w", err)
	}

	return true, info.IsDir(), nil
}

// Mkdir creates a directory
func (m *Manager) Mkdir(path string, parents bool) error {
	fullPath := m.resolvePath(path)

	if parents {
		return os.MkdirAll(fullPath, 0755)
	}

	return os.Mkdir(fullPath, 0755)
}

// ZipFiles creates a zip archive of files or directories
func (m *Manager) ZipFiles(sourcePath, destPath string, recursive bool) error {
	fullSourcePath := m.resolvePath(sourcePath)
	fullDestPath := m.resolvePath(destPath)

	// Create parent directories if needed
	parentDir := filepath.Dir(fullDestPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories: %w", err)
	}

	destFile, err := os.Create(fullDestPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer destFile.Close()

	zipWriter := zip.NewWriter(destFile)
	defer zipWriter.Close()

	info, err := os.Stat(fullSourcePath)
	if err != nil {
		return fmt.Errorf("failed to access source: %w", err)
	}

	if info.IsDir() {
		if !recursive {
			return fmt.Errorf("cannot zip directory without recursive flag")
		}
		return m.zipDirectory(zipWriter, fullSourcePath, filepath.Base(fullSourcePath))
	}

	return m.zipFile(zipWriter, fullSourcePath, filepath.Base(fullSourcePath))
}

// zipFile adds a single file to a zip archive
func (m *Manager) zipFile(zipWriter *zip.Writer, filePath, zipPath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, file)
	return err
}

// zipDirectory recursively adds a directory to a zip archive
func (m *Manager) zipDirectory(zipWriter *zip.Writer, dirPath, zipPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		fullPath := filepath.Join(dirPath, entry.Name())
		entryZipPath := filepath.Join(zipPath, entry.Name())

		if entry.IsDir() {
			// Create directory entry in zip
			_, err := zipWriter.Create(entryZipPath + "/")
			if err != nil {
				return err
			}

			if err := m.zipDirectory(zipWriter, fullPath, entryZipPath); err != nil {
				return err
			}
		} else {
			if err := m.zipFile(zipWriter, fullPath, entryZipPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// UnzipFiles extracts a zip archive
func (m *Manager) UnzipFiles(sourcePath, destPath string) error {
	fullSourcePath := m.resolvePath(sourcePath)
	fullDestPath := m.resolvePath(destPath)

	// Create destination directory
	if err := os.MkdirAll(fullDestPath, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	reader, err := zip.OpenReader(fullSourcePath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	for _, file := range reader.File {
		destFilePath := filepath.Join(fullDestPath, file.Name)

		// Check for Zip Slip vulnerability
		if !strings.HasPrefix(filepath.Clean(destFilePath), filepath.Clean(fullDestPath)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(destFilePath, 0755)
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
			return err
		}

		// Extract file
		destFile, err := os.OpenFile(destFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		srcFile, err := file.Open()
		if err != nil {
			destFile.Close()
			return err
		}

		_, err = io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()

		if err != nil {
			return err
		}

		// Set modification time
		os.Chtimes(destFilePath, time.Now(), file.ModTime())
	}

	return nil
}

// resolvePath resolves a relative path to an absolute path within the base directory
func (m *Manager) resolvePath(path string) string {
	// Clean the path to prevent directory traversal
	cleanPath := filepath.Clean(path)

	// Remove leading slash if present
	cleanPath = strings.TrimPrefix(cleanPath, "/")

	// Join with base path
	fullPath := filepath.Join(m.basePath, cleanPath)

	// Ensure the path is within the base directory
	absBase, _ := filepath.Abs(m.basePath)
	absPath, _ := filepath.Abs(fullPath)

	if !strings.HasPrefix(absPath, absBase) {
		// Path traversal attempt, return base path
		return m.basePath
	}

	return fullPath
}

// GetBasePath returns the base path for file operations
func (m *Manager) GetBasePath() string {
	return m.basePath
}
