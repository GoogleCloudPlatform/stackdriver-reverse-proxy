package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/trace"
)

// TODO(jbd): Support HTTPS.

var (
	projectID     string
	listen        string
	target        string
	tlsCert       string
	tlsKey        string
	enableLogging bool
)

func main() {
	ctx := context.Background()

	flag.StringVar(&projectID, "project", "", "google cloud project ID")
	flag.StringVar(&listen, "http", ":6996", "host:port proxy listens")
	flag.StringVar(&target, "target", "", "target server")
	flag.StringVar(&tlsCert, "tls-cert", "", "TLS cert file to start an HTTPS proxy")
	flag.StringVar(&tlsKey, "tls-key", "", "TLS key file to start an HTTPS proxy")
	flag.BoolVar(&enableLogging, "enable-logging", false, "set to enable logging to stackdriver")
	flag.Parse()

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
	// TODO(jbd): Show usage if projectID is not set.

	tc, err := trace.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Cannot initiate trace client: %v", err)
	}
	sp, _ := trace.NewLimitedSampler(1, 1<<32)
	tc.SetSamplingPolicy(sp)

	url, err := url.Parse(target)
	if err != nil {
		log.Fatal(err)
	}

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
	defer s.FinishWait()

	resp, err := http.DefaultTransport.RoundTrip(req)
	return resp, err
}
