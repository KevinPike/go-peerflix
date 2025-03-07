package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/dustin/go-humanize"
)

var t torrent.Torrent
var seed *bool
var vlc *bool
var progress int64

const clearScreen = "\033[H\033[2J"

// Exit statuses.
const (
	_                       = iota
	exitNoTorrentProvided   = iota
	exitErrorCreatingClient = iota
	exitErrorAddingTorrent  = iota
)

func main() {
	// Set up flags.
	seed = flag.Bool("seed", true, "Seed after finished downloading")
	vlc = flag.Bool("vlc", false, "Open vlc to play the file")

	flag.Parse()
	if len(flag.Args()) == 0 {
		usage()
		os.Exit(exitNoTorrentProvided)
	}

	// Start up the torrent client.
	client, err := torrent.NewClient(&torrent.Config{
		DataDir:  os.TempDir(),
		NoUpload: !(*seed),
	})

	if err != nil {
		log.Fatal(err)
		os.Exit(exitErrorCreatingClient)
	}

	// Add the magnet url.
	if t, err = client.AddMagnet(flag.Arg(0)); err != nil {
		log.Fatal(err)
		os.Exit(exitErrorAddingTorrent)
	}

	// Start downloading files.
	go func() {
		<-t.GotInfo()
		t.DownloadAll()
	}()

	// Http handler.
	go func() {
		http.HandleFunc("/", getFile)
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	if *vlc {
		go func() {
			for !readyForPlayback() {
				time.Sleep(time.Second)
			}
			log.Printf("Playing in vlc")

			// @todo decide command to run based on os.
			exec.Command("open", "-a", "vlc", "http://localhost:8080").Start()
		}()
	}

	// Cli render loop.
	for true {
		render()
		time.Sleep(time.Second)
	}
}

func readyForPlayback() bool {
	percentage := float64(t.BytesCompleted()) / float64(t.Length())

	return percentage > 0.05
}

func render() {
	var currentProgress = t.BytesCompleted()
	speed := humanize.Bytes(uint64(currentProgress-progress)) + "/s"
	progress = currentProgress

	percentage := float64(t.BytesCompleted()) / float64(t.Length()) * 100
	complete := humanize.Bytes(uint64(t.BytesCompleted()))
	size := humanize.Bytes(uint64(t.Length()))
	connections := len(t.Conns)

	print(clearScreen)
	fmt.Println(t.Name())
	fmt.Println("=============================================================")
	if t.BytesCompleted() > 0 {
		fmt.Printf("Progress: \t%s / %s  %.2f%%\n", complete, size, percentage)
	}
	if t.BytesCompleted() < t.Length() {
		fmt.Printf("Download speed: %s\n", speed)
	}
	fmt.Printf("Connections: \t%d\n", connections)
}

func usage() {
	flag.Usage()
}

func getLargestFile() torrent.File {
	var target torrent.File
	var maxSize int64

	for _, file := range t.Files() {
		if maxSize < file.Length() {
			maxSize = file.Length()
			target = file
		}
	}

	return target
}

func getFile(w http.ResponseWriter, r *http.Request) {
	target := getLargestFile()
	entry, err := NewFileReader(target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer entry.Close()

	w.Header().Set("Content-Disposition", "attachment; filename=\""+t.Name()+"\"")
	http.ServeContent(w, r, target.DisplayPath(), time.Now(), entry)
}
