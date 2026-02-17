//TODO: Allow for download from multiple sources. File will be downloaded in parts and then concatenated
//TODO: bittorrent protocol? Probably want a library for this

package main

import (
	"DLError"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"time"

	"io/ioutil"

	"strings"

	"github.com/cheggaaa/pb"
)

// Config holds the configuration for a download
type Config struct {
	URL         string
	FilePath    string
	Resume      bool
	Timeout     time.Duration
	Checksum    string
	ChecksumAlg string // "md5", "sha256", "sha512"
	MaxRetries  int
	Quiet       bool
}

var usage = `
dl - HTTP file downloader with resume capability

Usage: dl -url "http://url" [options]

Options:
  -url string        URL to download (required)
  -o string          Output file path (auto-detected if not specified)
  -r                 Resume incomplete download (requires -o)
  -timeout int       Request timeout in seconds (default: 30)
  -retry int         Maximum retry attempts (default: 3)
  -q                 Quiet mode - no progress bar
  -md5 string        Expected MD5 checksum for verification
  -sha256 string     Expected SHA256 checksum for verification
  -sha512 string     Expected SHA512 checksum for verification

Examples:
  dl -url "http://example.com/file.zip"
  dl -url "http://example.com/file.zip" -o output.zip -sha256 "abc123..."
  dl -url "http://example.com/file.zip" -o output.zip -r
  dl -url "http://example.com/file.zip" -retry 5 -timeout 60
`

func parseFlags() (*Config, error) {
	urlFlag := flag.String("url", "", "the url to download")
	filePath := flag.String("o", "", "the output file path")
	resume := flag.Bool("r", false, "-r")
	timeout := flag.Int("timeout", 30, "request timeout in seconds")
	md5sum := flag.String("md5", "", "expected MD5 checksum")
	sha256sum := flag.String("sha256", "", "expected SHA256 checksum")
	sha512sum := flag.String("sha512", "", "expected SHA512 checksum")
	maxRetries := flag.Int("retry", 3, "maximum number of retry attempts")
	quiet := flag.Bool("q", false, "quiet mode (no progress bar)")

	flag.Parse()

	if *urlFlag == "" {
		return nil, errors.New("URL is not set")
	}
	if *resume && *filePath == "" {
		return nil, errors.New("-o must be set if you are resuming")
	}

	// Validate URL
	_, err := url.ParseRequestURI(*urlFlag)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Determine checksum algorithm
	checksum := ""
	checksumAlg := ""
	if *sha256sum != "" {
		checksum = *sha256sum
		checksumAlg = "sha256"
	} else if *sha512sum != "" {
		checksum = *sha512sum
		checksumAlg = "sha512"
	} else if *md5sum != "" {
		checksum = *md5sum
		checksumAlg = "md5"
	}

	return &Config{
		URL:         *urlFlag,
		FilePath:    *filePath,
		Resume:      *resume,
		Timeout:     time.Duration(*timeout) * time.Second,
		Checksum:    checksum,
		ChecksumAlg: checksumAlg,
		MaxRetries:  *maxRetries,
		Quiet:       *quiet,
	}, nil
}

func main() {
	config, err := parseFlags()
	if err != nil {
		fmt.Println(err)
		fmt.Println(usage)
		return
	}

	err = downloadWithRetry(config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// downloadWithRetry implements retry logic with exponential backoff
func downloadWithRetry(config *Config) error {
	var lastErr error
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2^attempt seconds
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if !config.Quiet {
				fmt.Printf("Retry attempt %d/%d after %v...\n", attempt, config.MaxRetries, backoff)
			}
			time.Sleep(backoff)
		}
		
		err := downloadFile(config)
		if err == nil {
			// Download successful, verify checksum if provided
			if config.Checksum != "" {
				if !config.Quiet {
					fmt.Printf("Verifying %s checksum...\n", config.ChecksumAlg)
				}
				err = verifyChecksum(config.FilePath, config.Checksum, config.ChecksumAlg)
				if err != nil {
					return fmt.Errorf("checksum verification failed: %w", err)
				}
				if !config.Quiet {
					fmt.Println("✓ Checksum verified successfully")
				}
			}
			return nil
		}
		
		lastErr = err
		
		// Don't retry on certain errors
		if isNonRetryableError(err) {
			return err
		}
	}
	
	return fmt.Errorf("download failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// isNonRetryableError determines if an error should not be retried
func isNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Don't retry on client errors (4xx) except 408 (timeout)
	return strings.Contains(errStr, "status code 4") && !strings.Contains(errStr, "408")
}

func downloadFile(config *Config) error {
	client := &http.Client{
		Timeout: config.Timeout,
	}

	req, err := http.NewRequest("GET", config.URL, nil)
	if err != nil {
		return DLError.New("Creating request error", err)
	}

	var resp *http.Response
	filePath := config.FilePath

	//No resume
	if !config.Resume {
		resp, err = client.Do(req)
		if err != nil {
			return DLError.New("GET Request Error", err)
		}
		defer resp.Body.Close()

		// Validate HTTP status code
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP error: %s (status code %d)", resp.Status, resp.StatusCode)
		}

		if filePath == "" {
			filePath = extractFilename(resp, config.URL)
		}
	}

	//Open or Create file
	f, offset, err := OpenFile(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	//With Resume
	if offset > 0 && config.Resume {
		if !config.Quiet {
			fmt.Println("Resuming download...")
		}
		//Do the request again with a Range header so as not to download everything again
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", offset))
		resp, err = client.Do(req)
		if err != nil {
			return DLError.New("GET Request Error", err)
		}
		defer resp.Body.Close()

		// Check if server supports Range requests
		if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			return fmt.Errorf("resume failed: file already complete or server range error")
		}

		if resp.StatusCode != http.StatusPartialContent {
			if resp.StatusCode == http.StatusOK {
				// Server doesn't support Range - need to start over
				if !config.Quiet {
					fmt.Println("Warning: Server doesn't support resume. Starting download from beginning...")
				}
				f.Close()
				os.Remove(filePath)
				// Recursively call with Resume disabled
				newConfig := *config
				newConfig.Resume = false
				return downloadFile(&newConfig)
			}
			return fmt.Errorf("HTTP error: %s (status code %d)", resp.Status, resp.StatusCode)
		}

		// Verify server actually supports Range
		if len(resp.Header.Get("Content-Range")) == 0 {
			if !config.Quiet {
				fmt.Println("Warning: Server doesn't support Range header properly. Starting from beginning...")
			}
			f.Close()
			os.Remove(filePath)
			newConfig := *config
			newConfig.Resume = false
			return downloadFile(&newConfig)
		}
	}

	if resp == nil {
		return DLError.New("resp is not set...", errors.New("No response set"))
	}

	contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	totalSize := contentLength + int(offset)

	if !config.Quiet {
		fmt.Printf("Downloading to: %s\n", filePath)
	}

	if config.Quiet {
		// Quiet mode: just copy without progress bar
		_, err = io.Copy(f, resp.Body)
		return err
	}

	// Show progress bar
	bar := pb.New(totalSize).SetUnits(pb.U_BYTES)
	bar.Start()
	bar.SetRefreshRate(time.Millisecond * 100)
	bar.ShowPercent = true

	bar.ShowTimeLeft = true
	bar.ShowSpeed = true

	bar.Set(int(offset))
	r := bar.NewProxyReader(resp.Body)
	io.Copy(f, r)

	bar.Finish()

	return nil
}

// verifyChecksum computes and verifies the file checksum
func verifyChecksum(filePath, expectedChecksum, algorithm string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file for checksum: %w", err)
	}
	defer f.Close()

	var h hash.Hash
	switch algorithm {
	case "md5":
		h = md5.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return fmt.Errorf("unsupported checksum algorithm: %s", algorithm)
	}

	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("error reading file for checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(h.Sum(nil))
	expectedChecksum = strings.ToLower(strings.TrimSpace(expectedChecksum))

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

// extractFilename extracts filename from Content-Disposition header or URL
func extractFilename(resp *http.Response, downloadURL string) string {
	// Try Content-Disposition header first
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		// Look for filename= or filename*= parameter
		if strings.Contains(cd, "filename=") {
			parts := strings.Split(cd, "filename=")
			if len(parts) > 1 {
				filename := strings.Trim(parts[1], `"`)
				filename = strings.Split(filename, ";")[0]
				if filename != "" {
					return strings.TrimSpace(filename)
				}
			}
		}
	}

	// Fallback to URL path
	parsedURL, err := url.Parse(downloadURL)
	if err == nil && parsedURL.Path != "" {
		parts := strings.Split(parsedURL.Path, "/")
		filename := parts[len(parts)-1]
		if filename != "" {
			return filename
		}
	}

	// Last resort
	return "downloaded_file"
}

//OpenFile will open an existing file and seek to the end
func OpenFile(filePath string) (*os.File, int64, error) {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0666)
	offset := int64(0)
	if err != nil {
		f, err = os.Create(filePath)
		if err != nil {
			return nil, 0, DLError.New("Creating file error", err)
		}
		return f, 0, nil
	}

	fi, err := f.Stat()
	if err != nil {
		return nil, 0, DLError.New("Getting file stat error", err)
	}
	offset = fi.Size()
	return f, offset, nil
}
