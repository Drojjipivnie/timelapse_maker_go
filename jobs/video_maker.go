package jobs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"log"
	"net"
	"os"
	"path/filepath"
	Mregexp "regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"timelapse_maker/constants"
)

var Continue = &ProgressStatus{"continue"}
var End = &ProgressStatus{"end"}
var bitrateRegex = Mregexp.MustCompile(`(\d+(?:\.\d+)?)kbits/s`)

type FFMpegProgress struct {
	Frame      uint16
	Fps        string
	Bitrate    uint32 //bit per second
	TotalSize  uint32 //bytes
	OutTimeMs  uint32 //millis passed
	DupFrames  uint8
	DropFrames uint8
	Speed      string
	Status     *ProgressStatus
}

type ProgressStatus struct {
	Name string
}

type VideoMakerJob struct {
	RootDirectory       string
	ImagesRootDirectory string
	TimelapseType       *constants.TimelapseType
	DBPool              *pgxpool.Pool
	ProgressListener    func(p FFMpegProgress)
}

func (g VideoMakerJob) Run() {
	listener, socketError := net.Listen("tcp", "127.0.0.1:0")
	if socketError != nil {
		log.Printf("Error while opening socket for listening ffmpeg progress")
	} else {
		defer listener.Close()
		log.Println("Preparing to listen ffmpeg progress on ", listener.Addr().String())
		go g.handleFfmpegProgress(listener)
	}

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

	log.Printf("Prepared Frame order in file %s", file)
	targetVideoDirectory := filepath.Join(g.RootDirectory, g.TimelapseType.Directory, subDirectoryName)
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

	inputArgs := ffmpeg.KwArgs{"r": "5/1", "safe": 0, "f": "concat"}
	if socketError == nil {
		inputArgs["progress"] = "tcp://" + listener.Addr().String()
	}
	outputArgs := ffmpeg.KwArgs{"crf": 28, "s": "1280x720", "vcodec": "libx265"}

	err = ffmpeg.Input(file, inputArgs).Output(videoFilePath, outputArgs).OverWriteOutput().Run()
	if err != nil {
		log.Print("Error while creating video from images")
		return
	}
	videoCreated = true
	log.Printf("Finished creating video from images to %s", videoFilePath)
}

func (g VideoMakerJob) handleFfmpegProgress(lis net.Listener) {
	c, err := lis.Accept()
	if err != nil {
		log.Printf("Error while accepting connection using %s : %v", lis.Addr().String(), err)
		return
	}
	defer c.Close()

	log.Printf("Serving %s", c.RemoteAddr().String())
	buffer := bufio.NewReader(c)
	var p = FFMpegProgress{}
	for {
		netData, err := buffer.ReadString('\n')
		if err != nil {
			log.Printf("Got EOF or something else while reading from stream: %v", err)
			return
		}
		if p.parseLine(netData) {
			if g.ProgressListener != nil {
				g.ProgressListener(p)
			}
			p = FFMpegProgress{}
		}
	}
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

func (p *FFMpegProgress) parseLine(in string) bool {
	trimmed := strings.TrimSpace(in)
	if len(trimmed) == 0 {
		return false
	}

	splitValues := strings.Split(trimmed, "=")
	if len(splitValues) != 2 {
		return false
	}

	key := splitValues[0]
	value := splitValues[1]
	switch key {
	case "frame":
		frameAsInt, _ := strconv.Atoi(value)
		p.Frame = uint16(frameAsInt)
		return false
	case "fps":
		p.Fps = value
		return false
	case "bitrate":
		if value == "N/A" {
			p.Bitrate = 0
		} else {
			match := bitrateRegex.FindStringSubmatch(value)
			if len(match[1]) != 0 {
				float, _ := strconv.ParseFloat(match[1], 16)
				p.Bitrate = uint32(float * 1000)
			} else {
				p.Bitrate = 0
			}
		}
		return false
	case "total_size":
		if value == "N/A" {
			p.TotalSize = 0
		} else {
			parseInt, _ := strconv.ParseInt(value, 10, 32)
			p.TotalSize = uint32(parseInt)
		}
		return false
	case "out_time_ms":
		if value[0] == '-' {
			p.OutTimeMs = 0
		} else {
			parseInt, _ := strconv.ParseInt(value, 10, 32)
			p.OutTimeMs = uint32(parseInt)
		}
		return false
	case "dup_frames":
		parseInt, _ := strconv.ParseInt(value, 10, 8)
		p.DupFrames = uint8(parseInt)
		return false
	case "drop_frames":
		parseInt, _ := strconv.ParseInt(value, 10, 8)
		p.DropFrames = uint8(parseInt)
		return false
	case "speed":
		p.Speed = value
		return false
	case "progress":
		if value == "continue" {
			p.Status = Continue
		} else if value == "end" {
			p.Status = End
		}
		return true
	default:
		return false
	}
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
	abs, _ := filepath.Abs(imagesToCollectDirectory)
	for _, data := range dir {
		_, _ = writer.WriteString(fmt.Sprintf("file '%s'\n", filepath.Join(abs, data.Name())))
		_, _ = writer.WriteString("duration 0.2\n")
	}

	return temp.Name(), nil
}
