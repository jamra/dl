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
		fmt.Println(err.Error())
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
		fmt.Println("Downloading file...")
		resp, err = client.Do(req)
		if err != nil {
			return DLError.New("GET Request Error", err)
		}

		if filePath == "" {
			fmt.Println(resp.Header)
			cd := resp.Header.Get("Content-Disposition")
			if cd == "" {
				filePath = "file.png"
			} else {
				filePath = cd
			}
		}
	}

	//Open or Create file
	f, offset, err := OpenFile(filePath)

	//With Resume
	if offset > 0 && *resume == true {
		fmt.Println("Resuming download...")
		//Do the request again with a Content-Range so as not to download everything again
		req.Header.Add("Content-Range", fmt.Sprintf("%d", offset))
		resp, err = client.Do(req)
		if err != nil {
			return DLError.New("GET Request Error", err)
		}
	}

	if resp == nil {
		return DLError.New("resp is not set...", errors.New("No response set"))
	}

	fmt.Println("writing file named:", filePath)
	io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

//OpenFile will open an existing file and seek to the end
func OpenFile(filePath string) (*os.File, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		f, err = os.Create(filePath)
		if err != nil {
			return nil, 0, DLError.New("Creating file error", err)
		}
		return f, 0, nil
	}

	fi, err := f.Stat()

	_, err = f.Seek(fi.Size(), 2)
	if err != nil {
		return nil, fi.Size(), DLError.New("File seek error", err)
	}
	return f, fi.Size(), nil
}
