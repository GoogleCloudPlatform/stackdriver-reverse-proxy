package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

var (
	projectID = flag.String("project", "", "google cloud project ID")
	listen    = flag.String("http", ":6996", "host:port proxy listens")
	target    = flag.String("target", "", "target server")
)

// TODO(jbd): Support HTTPS.

func main() {
	flag.Parse()

	url, err := url.Parse(*target)
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Director = func(r *http.Request) {
		log.Println(r)
	}

	log.Fatal(http.ListenAndServe(*listen, proxy))
}
