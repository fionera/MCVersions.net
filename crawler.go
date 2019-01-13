package main

import (
	"context"
	"github.com/cenkalti/backoff"
	"github.com/sirupsen/logrus"
	"sync/atomic"
)

func crawler(c context.Context) {
	defer crawlerGroup.Done()

	for job := range jobs {
		if atomic.LoadInt32(&exitRequested) != 0 {
			break
		}

		err := backoff.Retry(func() error {
			err := job.Crawl(c)
			if err != nil {
				logrus.WithError(err).
					Errorf("Failed crawling")
			}

			return err
		}, backoff.NewExponentialBackOff())

		if err != nil {
			logrus.Fatal(err)
		}
	}
}
