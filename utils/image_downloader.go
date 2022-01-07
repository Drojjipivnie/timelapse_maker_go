package utils

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: time.Second * 10}

var mutex = sync.Mutex{}

var cachedDuration = time.Second * 30
var cachedBytes []byte = nil
var cachedTill = time.Now().Truncate(cachedDuration)

type ImageDownloader struct {
	Url            string
	SocketTimeout  int
	ConnectTimeout int
}

func (c *ImageDownloader) DownloadAsByteArray() (*[]byte, error) {
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now()
	if cachedBytes == nil || now.After(cachedTill) {
		log.Printf("GET to %s", c.Url)
		response, err := httpClient.Get(c.Url)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode == 200 {
			bytes, err := ioutil.ReadAll(response.Body)
			cachedBytes = bytes
			cachedTill = now.Add(cachedDuration)

			return &bytes, err
		} else {
			return nil, errors.New(fmt.Sprintf("got status %d", response.StatusCode))
		}
	} else {
		log.Print("Returning cached bytes")
		return &cachedBytes, nil
	}
}
