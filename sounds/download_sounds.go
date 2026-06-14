package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SoundLibrary struct {
	BaseURL    BaseURL `json:"baseUrl"`
	FileType   string  `json:"defaultFileType"`
	Sounds     []Sound `json:"sounds"`
}

type BaseURL struct {
	CloudFront string `json:"cloudFrontUrl"`
}

type Sound struct {
	AudioPath string `json:"audioFilePath"`
	Category  string `json:"category"`
	Duration  float64 `json:"duration"`
	Name      string `json:"name"`
}

type job struct {
	Sound
	url  string
	path string
}

const outputDir = "/hermes-data/sounds/downloaded"
const jsonPath = "/hermes-data/sounds/ask-soundlibrary.json"
const concurrency = 8

var client = &http.Client{Timeout: 60 * time.Second}

func main() {
	// Read JSON
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var lib SoundLibrary
	if err := json.Unmarshal(raw, &lib); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🎵 Alexa Sound Library Downloader\n")
	fmt.Printf("📦 Base: %s\n", lib.BaseURL.CloudFront)
	fmt.Printf("🔊 Total sounds: %d\n", len(lib.Sounds))
	fmt.Printf("📁 Output: %s\n", outputDir)
	fmt.Printf("⚡ Concurrency: %d\n\n", concurrency)

	os.MkdirAll(outputDir, 0755)

	// Download with worker pool
	jobs := make(chan job, len(lib.Sounds))
	results := make(chan string, len(lib.Sounds))

	// Start workers
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go worker(jobs, results, &wg)
	}

	// Feed jobs
	for _, s := range lib.Sounds {
		url := lib.BaseURL.CloudFront + s.AudioPath + "." + lib.FileType
		// Create output path: category dir / filename based on audioPath
		// audioPath e.g. "air/fire_extinguisher/fire_extinguisher_01"
		path := filepath.Join(outputDir, s.AudioPath + "." + lib.FileType)
		jobs <- job{Sound: s, url: url, path: path}
	}
	close(jobs)

	// Close results when workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Track progress
	downloaded := 0
	skipped := 0
	errors := 0
	catErrors := make(map[string]int)
	catOK := make(map[string]int)

	for res := range results {
		if strings.HasPrefix(res, "✅") {
			downloaded++
		} else if strings.HasPrefix(res, "⏭️") {
			skipped++
		} else if strings.HasPrefix(res, "❌") {
			errors++
			// Extract category from result
			parts := strings.SplitN(res, "|", 2)
			if len(parts) == 2 {
				catErrors[parts[1]]++
			}
		}

		// Show per-category progress for the last sound of each category
		// We do this via the result message
		if strings.Contains(res, "|") {
			parts := strings.SplitN(res, "|", 3)
			if len(parts) == 3 {
				catOK[parts[1]]++
			}
		}
		if strings.Contains(res, " ERR:") {
			fmt.Println(res)
		}
	}

	fmt.Println("\n\n============================================")
	fmt.Println("SUMMARY")
	fmt.Println("============================================")
	fmt.Printf("✅ Downloaded: %d\n", downloaded)
	fmt.Printf("⏭️  Already existed: %d\n", skipped)
	fmt.Printf("❌ Errors: %d\n", errors)

	// Per-category summary
	if len(catErrors) > 0 {
		fmt.Println("\n⚠️  Categories with errors:")
		for cat, count := range catErrors {
			fmt.Printf("  ❌ %s: %d errors\n", cat, count)
		}
	}
}

func worker(jobs <-chan job, results chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	for j := range jobs {
		// Check if file already exists
		os.MkdirAll(filepath.Dir(j.path), 0755)
		if _, err := os.Stat(j.path); err == nil {
			results <- fmt.Sprintf("⏭️ |%s| Already exists: %s", j.Category, j.Name)
			continue
		}

		// Download
		resp, err := client.Get(j.url)
		if err != nil {
			results <- fmt.Sprintf("❌ |%s| %s ERR: %v", j.Category, j.Name, err)
			continue
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			results <- fmt.Sprintf("❌ |%s| %s ERR: HTTP %d", j.Category, j.Name, resp.StatusCode)
			continue
		}

		out, err := os.Create(j.path)
		if err != nil {
			resp.Body.Close()
			results <- fmt.Sprintf("❌ |%s| %s ERR: %v", j.Category, j.Name, err)
			continue
		}

		written, err := io.Copy(out, resp.Body)
		out.Close()
		resp.Body.Close()

		if err != nil {
			os.Remove(j.path)
			results <- fmt.Sprintf("❌ |%s| %s ERR: %v", j.Category, j.Name, err)
			continue
		}
		if written == 0 {
			os.Remove(j.path)
			results <- fmt.Sprintf("❌ |%s| %s ERR: empty file", j.Category, j.Name)
			continue
		}

		results <- fmt.Sprintf("✅ |%s| %s (%.0fKB)", j.Category, j.Name, float64(written)/1024)
	}
}
