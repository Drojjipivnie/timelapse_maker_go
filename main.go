package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"timelapse_maker/constants"
	"timelapse_maker/cron"
	"timelapse_maker/jobs"
	"timelapse_maker/utils"
)

var (
	propertyManager     = &utils.PropertyManager{}
	imageDownloader     = &utils.ImageDownloader{Url: propertyManager.GetProperty(constants.ImageUrl)}
	dbPool              = initDataBasePool(propertyManager.GetProperty(constants.DBUrl))
	baseDirectory       = propertyManager.GetProperty(constants.BaseDirectory)
	imagesBaseDirectory = filepath.Join(baseDirectory, "images")

	downloadJobs = [4]struct {
		string
		jobs.ImageDownloadJob
	}{
		{"0 */2 8-20 ? * *", jobs.ImageDownloadJob{RootDirectory: imagesBaseDirectory, TimelapseType: &constants.Day, ImageDownloader: imageDownloader}},
		{"0 */15 8-20 ? * *", jobs.ImageDownloadJob{RootDirectory: imagesBaseDirectory, TimelapseType: &constants.Week, ImageDownloader: imageDownloader}},
		{"0 0 8-20 ? * *", jobs.ImageDownloadJob{RootDirectory: imagesBaseDirectory, TimelapseType: &constants.Month, ImageDownloader: imageDownloader}},
		{"0 0 8,12,16,20 * * ?", jobs.ImageDownloadJob{RootDirectory: imagesBaseDirectory, TimelapseType: &constants.Quarter, ImageDownloader: imageDownloader}},
	}

	loggingProgressListener = func(p jobs.FFMpegProgress) {
		log.Printf("Frame:%d; Fps:%s; Size:%s; Time Passed: %s; Status: %s", p.Frame, p.Fps, byteCountSI(p.TotalSize), (time.Duration(p.OutTimeMc) * time.Microsecond).String(), p.Status.Name)
	}
	videosBaseDirectory = filepath.Join(baseDirectory, "videos")
	videoJobs           = [4]struct {
		string
		jobs.VideoMakerJob
	}{
		{"0 20 22 ? * *", jobs.VideoMakerJob{RootDirectory: videosBaseDirectory, ImagesRootDirectory: imagesBaseDirectory, TimelapseType: &constants.Day, DBPool: dbPool, ProgressListener: loggingProgressListener}},
		{"0 15 22 ? * SUN", jobs.VideoMakerJob{RootDirectory: videosBaseDirectory, ImagesRootDirectory: imagesBaseDirectory, TimelapseType: &constants.Week, DBPool: dbPool, ProgressListener: loggingProgressListener}},
		{"0 10 22 L * ?", jobs.VideoMakerJob{RootDirectory: videosBaseDirectory, ImagesRootDirectory: imagesBaseDirectory, TimelapseType: &constants.Month, DBPool: dbPool, ProgressListener: loggingProgressListener}},
		{"0 5 22 L MAR,JUN,SEP,DEC ?", jobs.VideoMakerJob{RootDirectory: videosBaseDirectory, ImagesRootDirectory: imagesBaseDirectory, TimelapseType: &constants.Quarter, DBPool: dbPool, ProgressListener: loggingProgressListener}},
	}
)

func main() {
	defer func() {
		dbPool.Close()
		log.Print("Database pool closed")
	}()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	location, _ := time.LoadLocation("Europe/Moscow")

	c := cron.New(cron.WithLocation(location), cron.WithSeconds())

	for _, element := range downloadJobs {
		_, err := c.AddJob(element.string, element.ImageDownloadJob)
		if err != nil {
			log.Fatal(fmt.Sprintf("%s image job not created due to %s", element.ImageDownloadJob.TimelapseType.Name, err))
		}
	}
	for _, element := range videoJobs {
		_, err := c.AddJob(element.string, element.VideoMakerJob)
		if err != nil {
			log.Fatal(fmt.Sprintf("%s video job not created due to %s", element.VideoMakerJob.TimelapseType.Name, err))
		}
	}

	c.Start()

	log.Print("Started...")

	wait := make(chan os.Signal, 1)
	signal.Notify(wait, os.Interrupt, syscall.SIGTERM, os.Kill)

	log.Printf("Got signal %s", <-wait)
	log.Print("Exiting...")
}

func initDataBasePool(dbURL string) *pgxpool.Pool {
	pool, err := pgxpool.Connect(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Unable to connection to database: %v\n", err)
	}
	log.Print("Connected!")
	return pool
}

func byteCountSI(b uint32) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint32(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}
