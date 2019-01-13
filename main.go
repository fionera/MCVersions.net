package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
)

var startTime = time.Now()
var totalBytes int64
var numDownloaded int64
var crawlerGroup sync.WaitGroup
var exitRequested int32
var jobs chan Job

type Job interface {
	Crawl(c context.Context) error
}

func main() {
	if err := parseArgs(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.Info("Starting MCVersions.net Crawler")
	logrus.Info("  https://github.com/fionera/MCVersions.net/")

	c, cancel := context.WithCancel(context.Background())

	go listenCtrlC(cancel)
	go stats()

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	req.SetRequestURI("https://mcversions.net/")

	if err := fasthttp.Do(req, res); err != nil {
		logrus.Panic(err)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(res.Body()))
	if err != nil {
		logrus.Panic(err)
	}

	jobs = make(chan Job, doc.Find(".client").Size()+doc.Find(".server").Size())

	doc.Find(".client").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the band and title
		href, exist := s.Attr("href")
		version, _ := s.Parent().Parent().Attr("id")

		if exist {
			jobs <- &File{
				fileType: []byte("client"),
				version:  []byte(version),
				url:      []byte(href),
			}
		}
	})

	doc.Find(".server").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the band and title
		href, exist := s.Attr("href")
		version, _ := s.Parent().Parent().Attr("id")

		if exist {
			jobs <- &File{
				fileType: []byte("server"),
				version:  []byte(version),
				url:      []byte(href),
			}
		}
	})

	// Start downloaders
	crawlerGroup.Add(int(*concurrency))
	for i := 0; i < int(*concurrency); i++ {
		go crawler(c)
	}

	// Shutdown
	close(jobs)
	crawlerGroup.Wait()

	total := atomic.LoadInt64(&totalBytes)
	dur := time.Since(startTime).Seconds()

	logrus.WithFields(logrus.Fields{
		"total_bytes": total,
		"dur":         dur,
		"avg_rate":    float64(total) / dur,
	}).Info("Stats")
}

func listenCtrlC(cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	atomic.StoreInt32(&exitRequested, 1)
	cancel()
	fmt.Fprintln(os.Stderr, "\nWaiting for downloads to finish...")
	fmt.Fprintln(os.Stderr, "Press ^C again to exit instantly.")
	<-c
	fmt.Fprintln(os.Stderr, "\nKilled!")
	os.Exit(255)
}

func stats() {
	for range time.NewTicker(time.Second).C {
		total := atomic.LoadInt64(&totalBytes)
		dur := time.Since(startTime).Seconds()

		logrus.WithFields(logrus.Fields{
			"files":      numDownloaded,
			"total_bytes": totalBytes,
			"avg_rate":    fmt.Sprintf("%.0f", float64(total)/dur),
		}).Info("Stats")
	}
}
