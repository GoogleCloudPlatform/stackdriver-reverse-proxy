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
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	"go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
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
)

const usage = `stackdriver-reverse-proxy [opts...] -target=<host:port>

For example, to start at localhost:6996 to proxy requests to localhost:6060,
  $ stackdriver-reverse-proxy -target=http://localhost:6060

Options:
  -http           hostname:port to start the proxy server, by default localhost:6996.
  -target         hostname:port where the app server is running.
  -project        Google Cloud Platform project ID if running outside of GCP.

Tracing options:
  -trace-sampling     Tracing sampling fraction, between 0 and 1.0.

HTTPS options:
  -tls-cert TLS cert file to start an HTTPS proxy.
  -tls-key  TLS key file to start an HTTPS proxy.
`

func main() {
	flag.Usage = func() {
		fmt.Println(usage)
	}

	flag.StringVar(&projectID, "project", "", "")
	flag.StringVar(&listen, "http", ":6996", "host:port proxy listens")
	flag.StringVar(&target, "target", "", "target server")
	flag.Float64Var(&traceFrac, "trace-sampling", 1, "sampling fraction for tracing")
	flag.StringVar(&tlsCert, "tls-cert", "", "TLS cert file to start an HTTPS proxy")
	flag.StringVar(&tlsKey, "tls-key", "", "TLS key file to start an HTTPS proxy")
	flag.Parse()

	if target == "" {
		usageExit()
	}

	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID: projectID,
	})
	if err != nil {
		log.Fatal(err)
	}

	view.RegisterExporter(exporter)
	trace.RegisterExporter(exporter)
	view.Subscribe(ochttp.DefaultViews...)
	trace.SetDefaultSampler(trace.ProbabilitySampler(traceFrac))

	url, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Cannot URL parse -target: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = &ochttp.Transport{
		Propagation: &propagation.HTTPFormat{},
	}
	if tlsCert != "" && tlsKey != "" {
		log.Fatal(http.ListenAndServeTLS(listen, tlsCert, tlsKey, proxy))
	} else {
		log.Fatal(http.ListenAndServe(listen, proxy))
	}
}

func usageExit() {
	flag.Usage()
	os.Exit(1)
}
