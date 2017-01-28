//TODO: write file extention based on the Content-Type header
//TODO: Test resume download
//TODO: Add progress bar(s) for download progress
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
	"os"
	"strconv"

	"time"

	"io/ioutil"

	"strings"

	"github.com/cheggaaa/pb"
)

var url *string
var filePath *string
var resume *bool

var usage = `
        dl downloads files over http and allows for resume features.
        usage dl -url "http://url" [[-r] -o file.out]]
`

func parseFlags() error {
	url = flag.String("url", "", "the url to download")
	filePath = flag.String("o", "", "the output file path")
	resume = flag.Bool("r", false, "-r")

	flag.Parse()

	if *url == "" {
		return errors.New("URL is not set")
	}
	if *resume && *filePath == "" {
		return errors.New("-o must be set if you are resuming")
	}

	return nil
}

func main() {
	err := parseFlags()
	if err != nil {
		fmt.Println(err)
		fmt.Println(usage)
		return
	}

	err = downloadFile(*url, *filePath)
	if err != nil {
		fmt.Println(err)
	}
}

func downloadFile(url, filePath string) error {
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return DLError.New("Creating request error", err)
	}

	var resp *http.Response

	//No resume
	if *resume == false {
		resp, err = client.Do(req)
		if err != nil {
			return DLError.New("GET Request Error", err)
		}

		if filePath == "" {
			cd := resp.Header.Get("Content-Disposition")
			if cd == "" {
				filePath = "file"
			} else {
				filePath = strings.Split(cd, "/")[1]
			}
		}
	}

	//Open or Create file
	f, offset, err := OpenFile(filePath)
	defer f.Close()

	//With Resume
	if offset > 0 && *resume == true {
		fmt.Println("Resuming download...")
		//Do the request again with a Content-Range so as not to download everything again
		req.Header.Add("Range", fmt.Sprintf("bytes=%d-", offset))
		resp, err = client.Do(req)
		if err != nil {
			return DLError.New("GET Request Error", err)
		}

		if len(resp.Header.Get("Content-Range")) == 0 {
			io.CopyN(ioutil.Discard, resp.Body, offset)
		}
	}

	if resp == nil {
		return DLError.New("resp is not set...", errors.New("No response set"))
	}

	len, _ := strconv.Atoi(resp.Header.Get("Content-Length"))

	bar := pb.New(len).SetUnits(pb.U_BYTES)
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
