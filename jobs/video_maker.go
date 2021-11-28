package jobs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
	"timelapse_maker/constants"
)

type VideoMakerJob struct {
	RootDirectory       string
	ImagesRootDirectory string
	TimelapseType       *constants.TimelapseType
	DBPool              *pgxpool.Pool
}

func (g VideoMakerJob) Run() {
	now := time.Now()
	subDirectoryName := g.TimelapseType.SubDirectoryNaming(now)
	imagesToCollectDirectory := filepath.Join(
		g.ImagesRootDirectory,
		g.TimelapseType.Directory,
		subDirectoryName)

	file, err := createFrameOrderFile(imagesToCollectDirectory)
	if err != nil {
		log.Print(err)
		return
	}

	log.Printf("Prepared frame order in file %s", file)
	targetVideoDirectory := filepath.Join(g.RootDirectory, subDirectoryName)
	err = os.MkdirAll(targetVideoDirectory, os.ModePerm)
	if err != nil {
		log.Printf("Error while creating directory %s", targetVideoDirectory)
		return
	}

	videoFilePath := filepath.Join(targetVideoDirectory, "timelapse.mp4")
	var videoCreated = false
	defer func() {
		if videoCreated {
			if g.saveInformationToDatabase(videoFilePath) != nil {
				log.Print("Error while saving info to database")
			} else {
				log.Printf("Saved information about %s in database", videoFilePath)
				err2 := os.RemoveAll(imagesToCollectDirectory)
				if err2 != nil {
					log.Printf("Error while removing images from %s due to %v", imagesToCollectDirectory, err2)
				}
			}
		}
	}()
	log.Printf("Starting to creating video from images to %s", videoFilePath)
	err = ffmpeg.
		Input(file, ffmpeg.KwArgs{"r": "5/1", "safe": 0, "f": "concat"}).
		Output(videoFilePath, ffmpeg.KwArgs{"crf": 28, "s": "1280x720", "vcodec": "libx265"}).
		OverWriteOutput().
		Run()
	if err != nil {
		log.Print("Error while creating video from images")
		return
	}
	videoCreated = true
	log.Printf("Finished creating video from images to %s", videoFilePath)
}

func createFrameOrderFile(imagesToCollectDirectory string) (string, error) {
	dir, err := os.ReadDir(imagesToCollectDirectory)
	if err != nil {
		return "", err
	}

	if len(dir) == 0 {
		return "", errors.New(fmt.Sprintf("No files found at %s. Exiting", imagesToCollectDirectory))
	}

	sort.SliceStable(dir, func(i, j int) bool {
		file1, _ := time.Parse("02-01-2006 15_04_05.jpg", dir[i].Name())
		file2, _ := time.Parse("02-01-2006 15_04_05.jpg", dir[j].Name())
		return file1.Before(file2)
	})

	temp, err := os.CreateTemp("", "*.txt")
	if err != nil {
		return "", err
	}
	defer temp.Close()

	writer := bufio.NewWriter(temp)
	defer writer.Flush()
	for _, data := range dir {
		_, _ = writer.WriteString(fmt.Sprintf("file '%s'\n", filepath.Join(imagesToCollectDirectory, data.Name())))
		_, _ = writer.WriteString("duration 0.2\n")
	}

	return temp.Name(), nil
}

func (g VideoMakerJob) saveInformationToDatabase(path string) error {
	parent := filepath.Base(filepath.Dir(path))
	//Must exists
	abs, _ := filepath.Abs(path)

	conn, err := g.DBPool.Acquire(context.Background())
	if err != nil {
		log.Printf("Unable to acquire a database connection: %v\n", err)
		return err
	}
	defer conn.Release()

	row := conn.QueryRow(context.Background(),
		"INSERT INTO \"lig2-test\".videos (name, type, file_path, uploaded) VALUES ($1, $2, $3, $4) RETURNING id",
		parent, g.TimelapseType.Name, abs, false)
	var id uint64
	err = row.Scan(&id)
	if err != nil {
		log.Printf("Unable to INSERT: %v", err)
		return err
	}
	return nil
}
