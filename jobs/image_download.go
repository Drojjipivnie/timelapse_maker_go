package jobs

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
	"timelapse_maker/constants"
	"timelapse_maker/utils"
)

type ImageDownloadJob struct {
	RootDirectory   string
	TimelapseType   *constants.TimelapseType
	ImageDownloader *utils.ImageDownloader
}

func (g ImageDownloadJob) Run() {
	log.Printf("Started job %s", g.TimelapseType.Name)
	byteArray, err := g.ImageDownloader.DownloadAsByteArray()
	if err != nil {
		log.Printf("Error occured while loading image: %s", err.Error())
		return
	}
	now := time.Now()
	absoluteFilePath := filepath.Join(
		g.RootDirectory,
		g.TimelapseType.Directory,
		g.TimelapseType.SubDirectoryNaming(now),
		now.Format("02-01-2006 15_04_05.jpg"))
	file, err := create(absoluteFilePath)
	if err != nil {
		log.Printf("Error occured while touching file %s. %s", absoluteFilePath, err.Error())
	}

	bytesWritten, err := io.Copy(file, bytes.NewReader(byteArray))
	if err != nil {
		log.Printf("Error occured while saving image to file: %s", err.Error())
	} else {
		log.Printf("Saved image sized %d to %s", bytesWritten, absoluteFilePath)
	}
}

func create(p string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0770); err != nil {
		return nil, err
	}
	return os.Create(p)
}
