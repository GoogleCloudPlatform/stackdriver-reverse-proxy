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

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/trace"
)

var (
	projectID string
	creds     string

	listen    string
	target    string
	tlsCert   string
	tlsKey    string
	traceFrac float64

	enableErrorReports bool
)

const usage = `stackdriver-proxy [opts...] -http=<host:port> -target=<target_url>

For example, to start at localhost:8080 to proxy requests to localhost:6060,
  $ stackdriver-proxy -http=:8080 -target=http://localhost:6060

Options:
  -project        Google Cloud Platform project ID if running outside of GCP.
  -creds          Google Cloud credentials file. If not given, ADC is used.
  -enable-errors  Set true to enable error reporting to Stackdriver.

Tracing options:
  -trace-fraction Tracing sampling fraction, [0, 1.0].

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
	flag.StringVar(&creds, "credentials", "", "")

	flag.StringVar(&listen, "http", ":6996", "host:port proxy listens")
	flag.StringVar(&target, "target", "", "target server")
	flag.Float64Var(&traceFrac, "trace-fraction", 1, "sampling fraction for tracing")
	flag.BoolVar(&enableErrorReports, "enable-errorreports", false, "set to enable error reporting to stackdriver")
	flag.StringVar(&tlsCert, "tls-cert", "", "TLS cert file to start an HTTPS proxy")
	flag.StringVar(&tlsKey, "tls-key", "", "TLS key file to start an HTTPS proxy")
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

	tc, err := trace.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Cannot initiate trace client: %v", err)
	}
	sp, _ := trace.NewLimitedSampler(traceFrac, 1<<32)
	tc.SetSamplingPolicy(sp)

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = &transport{
		Trace: tc,
	}
	if tlsCert != "" && tlsKey != "" {
		log.Fatal(http.ListenAndServeTLS(listen, tlsCert, tlsKey, proxy))
	} else {
		log.Fatal(http.ListenAndServe(listen, proxy))
	}
}

type transport struct {
	Trace *trace.Client
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	s := t.Trace.SpanFromRequest(req)
	defer s.Finish()

	req.Header.Set("X-Cloud-Trace-Context", s.Header())
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		s.SetLabel("error", err.Error())
	}
	return resp, err
}

func usageExit() {
	flag.Usage()
	os.Exit(1)
}
