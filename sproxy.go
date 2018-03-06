// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sproxy

import (
	"context"
	"errors"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3"
)

type Event struct {
	StatusCode   int
	Request      bool
	ResponseSize int64
	StartTime    time.Time
	EndTime      time.Time
	TraceID      string
	Err          error
}

type MetricsReporter struct {
	mu            sync.Mutex
	currentEvents []*Event
	period        time.Duration

	cancelChan <-chan bool
	cancelFn   func() error

	service *monitoring.MetricClient
}

const defaultPeriod = 1 * time.Minute

func NewMetricsReporter(ctx context.Context, projectID string, period time.Duration) (*MetricsReporter, error) {
	service, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, err
	}
	if period <= 0 {
		period = defaultPeriod
	}
	cancelChan, cancelFn := makeCanceler()
	mr := &MetricsReporter{
		period:        period,
		currentEvents: make([]*Event, 0, 512), // Arbitrary value
		cancelChan:    cancelChan,
		cancelFn:      cancelFn,
		service:       service,
	}
	return mr, nil
}

func (mr *MetricsReporter) Close() error {
	if err := mr.service.Close(); err != nil {
		return err
	}
	if mr.cancelFn == nil {
		return nil
	}
	return mr.cancelFn()
}

func (mr *MetricsReporter) AddEvent(evt *Event) {
	mr.mu.Lock()
	mr.currentEvents = append(mr.currentEvents, evt)
	mr.mu.Unlock()
}

var errAlreadyClosed = errors.New("already closed")

func makeCanceler() (<-chan bool, func() error) {
	cancelChan := make(chan bool, 1)
	var cancelOnce sync.Once
	cancelFn := func() error {
		err := errAlreadyClosed
		cancelOnce.Do(func() {
			close(cancelChan)
			err = nil
		})
		return err
	}

	return cancelChan, cancelFn
}

// Do is a synchronous method that reports events
// at the end of every specified sampling period
// sending them to the StackDriver Monitoring API.
func (mr *MetricsReporter) Do() {
	i := uint64(0)
	for {
		select {
		case <-mr.cancelChan:
			return
		case <-time.After(mr.period):
			// Otherwise the duration has lapsed
		}

		i++
		drainedEvents := mr.drainEvents()

		go mr.reportEvents(drainedEvents, i)
	}
}

// drainEvents is a concurrent-safe method that
// creates a frozen copy of mr.currentEvents and
// then drains mr.currentEvents resetting it an empty slice.
func (mr *MetricsReporter) drainEvents() []*Event {
	mr.mu.Lock()
	drained := make([]*Event, len(mr.currentEvents))
	copy(drained, mr.currentEvents)
	mr.currentEvents = mr.currentEvents[:0]
	mr.mu.Unlock()
	return drained
}

func (mr *MetricsReporter) reportEvents(events []*Event, nIter uint64) {
	nResponses := 0
	nRequests := 0
	totalResponseDuration := time.Duration(0)
	responsesCounter := make(map[int]int)
	traceIDs := make([]string, 0, len(events))
	for _, evt := range events {
		traceIDs = append(traceIDs, evt.TraceID)
		switch evt.Request {
		case true:
			nRequests++
		default:
			statusIndex := evt.StatusCode / 100
			responsesCounter[statusIndex]++
			nResponses++
			totalResponseDuration += evt.EndTime.Sub(evt.StartTime)
		}
	}

	if nResponses <= 0 {
		nResponses = 1
	}

	// Calculate the average rate of different
	// XXX responses within the past duration.
	rateXXXResponsesMap := make(map[int]float64)
	for statusIndex, count := range responsesCounter {
		rateXXXResponsesMap[statusIndex] = 100.0 * (float64(count) / float64(nResponses))
	}

	stats := &InflightStats{
		NRequests:                      nRequests,
		TraceIDs:                       traceIDs,
		NthIteration:                   nIter,
		RateResponseStats:              rateXXXResponsesMap,
		AverageResponseDurationSeconds: float64(totalResponseDuration.Seconds()) / float64(nResponses),
	}
	mr.sendStats(stats)
}

type InflightStats struct {
	NRequests                      int
	TraceIDs                       []string
	NthIteration                   uint64
	RateResponseStats              map[int]float64
	AverageResponseDurationSeconds float64
}

func (mr *MetricsReporter) sendStats(stats *InflightStats) error {
	// service.
}
