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

// Program stackdriver-reverse-proxy provides a Stackdriver reverse
// proxy that creates traces for the incoming requests, logs request
// details, and reports errors.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/trace"

	sproxy "github.com/GoogleCloudPlatform/stackdriver-reverse-proxy"
)

var (
	projectID string

	listen    string
	target    string
	tlsCert   string
	tlsKey    string
	traceFrac float64

	disableMonitoring bool
	monitoringPeriod  string

	enableErrorReports bool
)

const usage = `stackdriver-reverse-proxy [opts...] -target=<host:port>

For example, to start at localhost:6996 to proxy requests to localhost:6060,
  $ stackdriver-reverse-proxy -target=http://localhost:6060

Options:
  -http           hostname:port to start the proxy server, by default localhost:6996.
  -target         hostname:port where the app server is running.
  -project        Google Cloud Platform project ID if running outside of GCP.

Tracing options:
  -trace-fraction    Tracing sampling fraction, between 0 and 1.0.
  -monitor-http-off  Turning on reporting HTTP stats to StackDriver Monitoring
  -monitor-period    The period at which StackDriver Monitoring reports are sent

HTTPS options:
  -tls-cert TLS cert file to start an HTTPS proxy.
  -tls-key  TLS key file to start an HTTPS proxy.
`

func main() {
	ctx := context.Background()
	flag.Usage = func() {
		fmt.Println(usage)
	}

	flag.StringVar(&projectID, "project", "", "")
	flag.StringVar(&listen, "http", ":6996", "host:port proxy listens")
	flag.StringVar(&target, "target", "", "target server")
	flag.Float64Var(&traceFrac, "trace-fraction", 1, "sampling fraction for tracing")
	flag.StringVar(&tlsCert, "tls-cert", "", "TLS cert file to start an HTTPS proxy")
	flag.StringVar(&tlsKey, "tls-key", "", "TLS key file to start an HTTPS proxy")
	flag.BoolVar(&disableMonitoring, "disable-monitoring", false, "send monitor reports to stackdriver Monitoring")
	flag.StringVar(&monitoringPeriod, "monitoring-period", "1m", "the period for stackdriver Monitoring")
	flag.Parse()

	if target == "" {
		usageExit()
	}
	if projectID == "" {
		// Try to retrieve it from metadata server.
		if metadata.OnGCE() {
			pid, err := metadata.ProjectID()
			if err != nil {
				log.Fatalf("Cannot get project ID from metadata server: %v", err)
			}
			projectID = pid
		}
	}
	if projectID == "" {
		usageExit()
	}

	url, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Cannot URL parse -target: %v", err)
	}

	var metrics *sproxy.MetricsReporter
	if !disableMonitoring {
		period, err := time.ParseDuration(monitoringPeriod)
		if err != nil {
			period = 1 * time.Minute
		}
		metrics, err = sproxy.NewMetricsReporter(ctx, projectID, period)
		if err != nil {
			log.Fatalf("Cannot create the metricsReporter: %v", err)
		}
		// Fire up the metrics reporter.
		go metrics.Do()

		// Close it once we are done.
		defer metrics.Close()
	}

	tc, err := trace.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Cannot initiate trace client: %v", err)
	}
	sp, _ := trace.NewLimitedSampler(traceFrac, 1<<32)
	tc.SetSamplingPolicy(sp)

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = &transport{
		Trace:           tc,
		MetricsReporter: metrics,
	}
	if tlsCert != "" && tlsKey != "" {
		log.Fatal(http.ListenAndServeTLS(listen, tlsCert, tlsKey, proxy))
	} else {
		log.Fatal(http.ListenAndServe(listen, proxy))
	}
}

type transport struct {
	Trace           *trace.Client
	MetricsReporter *sproxy.MetricsReporter
}

func (t *transport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	s := t.Trace.SpanFromRequest(req)
	defer s.Finish()

	req.Header.Set("X-Cloud-Trace-Context", s.Header())

	if mr := t.MetricsReporter; mr != nil {
		startTime := time.Now()
		traceID := s.TraceID()
		go mr.AddEvent(&sproxy.Event{
			Request:   true,
			TraceID:   traceID,
			StartTime: startTime,
			EndTime:   startTime,
		})

		// At the end, once we have a response,
		// capture its end metrics and fire off events.
		defer func() {
			statusCode := 500
			if resp != nil {
				statusCode = resp.StatusCode
			}
			// Run in a goroutine so that RoundTrip isn't blocked at all.
			go mr.AddEvent(&sproxy.Event{
				StartTime:  startTime,
				EndTime:    time.Now(),
				TraceID:    traceID,
				StatusCode: statusCode,
				Err:        err,
			})
		}()
	}

	resp, err = http.DefaultTransport.RoundTrip(req)
	if err != nil {
		s.SetLabel("error", err.Error())
	}
	return resp, err
}

func usageExit() {
	flag.Usage()
	os.Exit(1)
}
