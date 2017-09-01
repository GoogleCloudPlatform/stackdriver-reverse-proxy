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
	"errors"
	"sync"
	"time"
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
	currentEvents []*Event
	period        time.Duration

	cancelChan <-chan bool
	cancelFn   func() error

	// mu protects statsRecvFn
	mu          sync.RWMutex
	statsRecvFn func(*InflightStats) error
}

const defaultPeriod = 1 * time.Minute

func NewMetricsReporter(period time.Duration) (*MetricsReporter, error) {
	if period <= 0 {
		period = defaultPeriod
	}
	cancelChan, cancelFn := makeCanceler()
	mr := &MetricsReporter{
		period:        period,
		currentEvents: make([]*Event, 0, 512), // Arbitrary value
		cancelChan:    cancelChan,
		cancelFn:      cancelFn,
	}
	return mr, nil
}

func (mr *MetricsReporter) Close() error {
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
		var err error = errAlreadyClosed
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

		i += 1
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
			nRequests += 1
		default:
			statusIndex := evt.StatusCode / 100
			responsesCounter[statusIndex] += 1
			nResponses += 1
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
	mr.mu.RLock()
	recvFn := mr.statsRecvFn
	mr.mu.RUnlock()
	if recvFn == nil {
		return nil
	}
	return recvFn(stats)
}

func (mr *MetricsReporter) SetStatsReceiver(statsRecvFn func(*InflightStats) error) {
	mr.mu.Lock()
	mr.statsRecvFn = statsRecvFn
	mr.mu.Unlock()
}
