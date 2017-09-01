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

package sproxy_test

import (
	"encoding/json"
	"log"
	"math/rand"
	"testing"
	"time"

	sproxy "github.com/GoogleCloudPlatform/stackdriver-reverse-proxy"
)

func TestReporting(t *testing.T) {
	t.Skipf("This test is very long. Meant to be a usage simulation")
	mr, err := sproxy.NewMetricsReporter(250 * time.Millisecond)
	if err != nil {
		t.Fatalf("metricsReporter: %v", err)
	}
	mr.SetStatsReceiver(func(stats *sproxy.InflightStats) error {
		blob, _ := json.MarshalIndent(stats, "", "  ")
		log.Printf("statsIn: %s", blob)
		return nil
	})
	go mr.Do()
	defer mr.Close()

	n := 10000
	doneChan := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func(ii int) {
			defer func() {
				doneChan <- true
			}()
			startTime := time.Now()
			evt := &sproxy.Event{
				StartTime:  startTime,
				StatusCode: rand.Intn(600) + 100,
			}

			evt.Request = rand.Intn(37)%2 == 0
			evt.EndTime = time.Now()
			mr.AddEvent(evt)
		}(i)

		refreshTime := time.Duration((1 + rand.Intn(110))) * time.Millisecond
		<-time.After(refreshTime)
	}

	for i := 0; i < n; i++ {
		<-doneChan
	}
}
