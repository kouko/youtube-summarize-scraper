package transcriber

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kouko/youtube-summarize-scraper/config"
	"github.com/kouko/youtube-summarize-scraper/lang"
)

// TranscribeResult holds the output of a transcription.
type TranscribeResult struct {
	SRTContent string
	Language   string
	ModelUsed  string
}

// Transcriber orchestrates audio download and whisper transcription.
type Transcriber struct {
	whisperPath   string
	ffmpegPath    string
	ytdlpPath     string
	whisperConfig config.WhisperConfig
}

// NewTranscriber creates a Transcriber with the given tool paths and config.
func NewTranscriber(whisperPath, ffmpegPath, ytdlpPath string, cfg config.WhisperConfig) *Transcriber {
	return &Transcriber{
		whisperPath:   whisperPath,
		ffmpegPath:    ffmpegPath,
		ytdlpPath:     ytdlpPath,
		whisperConfig: cfg,
	}
}

// Transcribe downloads audio from videoURL, runs whisper transcription,
// and returns the resulting SRT content.
func (t *Transcriber) Transcribe(videoURL string, language string, outputDir string, filePrefix string, cookieArgs []string) (*TranscribeResult, error) {
	// 1. Download audio via yt-dlp.
	audioPath := filepath.Join(outputDir, filePrefix+"audio.wav")
	if err := t.downloadAudio(videoURL, audioPath, cookieArgs); err != nil {
		return nil, fmt.Errorf("downloading audio: %w", err)
	}

	// 2. Select whisper model based on language.
	normalized := lang.NormalizeToISO639_1(language)
	modelName := t.whisperConfig.DefaultModel
	if m, ok := t.whisperConfig.LanguageModels[normalized]; ok {
		modelName = m
	}

	// 3. Resolve model path.
	modelPath := t.resolveModelPath(modelName)

	// 4. Download model if it doesn't exist.
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		log.Printf("Model file not found at %s, downloading %q ...", modelPath, modelName)
		if err := t.downloadModel(modelName, modelPath); err != nil {
			return nil, fmt.Errorf("downloading model: %w", err)
		}
	}

	// 5. Run whisper-cli with 30-minute timeout.
	srtBase := filepath.Join(outputDir, filePrefix+"whisper")
	if err := t.runWhisper(modelPath, audioPath, srtBase, normalized); err != nil {
		return nil, fmt.Errorf("running whisper: %w", err)
	}

	// 6. Read the generated .srt file.
	srtPath := srtBase + ".srt"
	srtData, err := os.ReadFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("reading SRT file %s: %w", srtPath, err)
	}

	// 7. Clean up the audio file.
	if err := os.Remove(audioPath); err != nil {
		log.Printf("Warning: failed to remove audio file %s: %v", audioPath, err)
	}

	return &TranscribeResult{
		SRTContent: string(srtData),
		Language:   normalized,
		ModelUsed:  modelName,
	}, nil
}

// downloadAudio uses yt-dlp to download and convert audio to 16kHz mono WAV.
func (t *Transcriber) downloadAudio(videoURL string, outputPath string, cookieArgs []string) error {
	ffmpegDir := filepath.Dir(t.ffmpegPath)

	args := []string{
		"-x",
		"--audio-format", "wav",
		"--postprocessor-args", "ffmpeg:-ar 16000 -ac 1",
		"--ffmpeg-location", ffmpegDir,
		"-o", outputPath,
	}
	args = append(args, cookieArgs...)
	args = append(args, videoURL)

	timeout := time.Duration(t.whisperConfig.DownloadTimeout) * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.ytdlpPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("Downloading audio: %s %v", t.ytdlpPath, args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp failed: %w", err)
	}
	return nil
}

// runWhisper executes whisper-cli to transcribe the audio file to SRT.
func (t *Transcriber) runWhisper(modelPath string, audioPath string, outputBase string, language string) error {
	args := []string{
		"-m", modelPath,
		"-f", audioPath,
		"-osrt",
		"-of", outputBase,
	}

	// Pass language hint to whisper for better accuracy.
	if language != "" {
		args = append(args, "-l", language)
	} else {
		args = append(args, "-l", "auto")
	}

	timeout := time.Duration(t.whisperConfig.TranscribeTimeout) * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.whisperPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("Running whisper: %s %v", t.whisperPath, args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("whisper-cli failed: %w", err)
	}
	return nil
}
