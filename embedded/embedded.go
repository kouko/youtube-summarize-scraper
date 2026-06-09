package embedded

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed bin
var binFS embed.FS

const cacheDir = ".ytss/bin"

// BinPaths holds resolved paths to embedded tool binaries.
type BinPaths struct {
	YtDlp      string
	FFmpeg     string
	WhisperCLI string
}

// ExtractAll extracts all embedded binaries to the cache directory (~/.ytss/bin/)
// and returns their paths. Skips extraction if the binary already exists with matching content.
func ExtractAll() (*BinPaths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	destDir := filepath.Join(homeDir, cacheDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	platform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	srcDir := filepath.Join("bin", platform)

	tools := map[string]string{
		"yt-dlp":      "",
		"ffmpeg":      "",
		"whisper-cli": "",
	}

	for name := range tools {
		srcPath := filepath.Join(srcDir, name)
		destPath := filepath.Join(destDir, name)

		if err := extractBinary(srcPath, destPath); err != nil {
			return nil, fmt.Errorf("extracting %s: %w", name, err)
		}
		tools[name] = destPath
	}

	return &BinPaths{
		YtDlp:      tools["yt-dlp"],
		FFmpeg:     tools["ffmpeg"],
		WhisperCLI: tools["whisper-cli"],
	}, nil
}

func extractBinary(srcPath, destPath string) error {
	srcData, err := binFS.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading embedded file %s: %w (did you run 'make deps'?)", srcPath, err)
	}

	srcHash := sha256.Sum256(srcData)

	// Check if destination already exists with matching hash
	if existingData, err := os.ReadFile(destPath); err == nil {
		existingHash := sha256.Sum256(existingData)
		if srcHash == existingHash {
			slog.Debug("binary already up to date", "path", destPath)
			return nil
		}
		slog.Info("updating binary", "path", destPath)
	}

	if err := os.WriteFile(destPath, srcData, 0755); err != nil {
		return fmt.Errorf("writing %s: %w", destPath, err)
	}

	slog.Info("extracted binary", "path", destPath, "size", len(srcData))
	return nil
}

// ListEmbedded returns the list of embedded files for the current platform.
func ListEmbedded() ([]string, error) {
	platform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	srcDir := filepath.Join("bin", platform)

	var files []string
	err := fs.WalkDir(binFS, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
