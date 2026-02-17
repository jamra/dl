//TODO: Allow for download from multiple sources. File will be downloaded in parts and then concatenated
//TODO: bittorrent protocol? Probably want a library for this

package main

import (
	"DLError"
	"errors"
	"flag"
	"fmt"
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
	URL      string
	FilePath string
	Resume   bool
	Timeout  time.Duration
}

var usage = `
        dl downloads files over http and allows for resume features.
        usage dl -url "http://url" [[-r] -o file.out]]
`

func parseFlags() (*Config, error) {
	urlFlag := flag.String("url", "", "the url to download")
	filePath := flag.String("o", "", "the output file path")
	resume := flag.Bool("r", false, "-r")
	timeout := flag.Int("timeout", 30, "request timeout in seconds")

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

	return &Config{
		URL:      *urlFlag,
		FilePath: *filePath,
		Resume:   *resume,
		Timeout:  time.Duration(*timeout) * time.Second,
	}, nil
}

func main() {
	config, err := parseFlags()
	if err != nil {
		fmt.Println(err)
		fmt.Println(usage)
		return
	}

	err = downloadFile(config)
	if err != nil {
		fmt.Println(err)
	}
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
		fmt.Println("Resuming download...")
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
				fmt.Println("Warning: Server doesn't support resume. Starting download from beginning...")
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
			fmt.Println("Warning: Server doesn't support Range header properly. Starting from beginning...")
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

	fmt.Printf("Downloading to: %s\n", filePath)

	bar := pb.New(totalSize).SetUnits(pb.U_BYTES)
	bar.Start()
	bar.SetRefreshRate(time.Millisecond * 100)
	bar.ShowPercent = false

	bar.ShowTimeLeft = true
	bar.ShowSpeed = true

	bar.Set(int(offset))
	r := bar.NewProxyReader(resp.Body)
	io.Copy(f, r)

	bar.Finish()

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
