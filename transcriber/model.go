package transcriber

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// knownCustomPrefixes lists model name prefixes that use "ggml-model.bin"
// instead of the standard "ggml-{name}.bin" naming convention.
var knownCustomPrefixes = []string{
	"belle-zh",
	"kotoba-ja",
}

// resolveModelPath returns the full filesystem path where the model file
// should be located. ModelDir is already expanded by config.Load().
func (t *Transcriber) resolveModelPath(modelName string) string {
	modelDir := t.whisperConfig.ModelDir

	// Determine filename: custom models use "ggml-model.bin" in a subdirectory,
	// standard models use "ggml-{name}.bin".
	for _, prefix := range knownCustomPrefixes {
		if strings.HasPrefix(modelName, prefix) {
			return filepath.Join(modelDir, modelName, "ggml-model.bin")
		}
	}

	return filepath.Join(modelDir, fmt.Sprintf("ggml-%s.bin", modelName))
}

// downloadModel downloads a whisper model from the configured source URL
// and saves it to destPath. It creates parent directories as needed.
func (t *Transcriber) downloadModel(modelName string, destPath string) error {
	url, ok := t.whisperConfig.ModelSources[modelName]
	if !ok {
		return fmt.Errorf("no download URL configured for model %q", modelName)
	}

	// Create parent directory if needed.
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating model directory %s: %w", dir, err)
	}

	log.Printf("Downloading whisper model %q from %s ...", modelName, url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("downloading model %q: %w", modelName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading model %q: HTTP %d", modelName, resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating model file %s: %w", destPath, err)
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("writing model file %s: %w", destPath, err)
	}

	log.Printf("Downloaded model %q: %d bytes -> %s", modelName, written, destPath)
	return nil
}
