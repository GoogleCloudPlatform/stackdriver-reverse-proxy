# stackdriver-reverse-proxy

[![Build Status](https://travis-ci.org/GoogleCloudPlatform/stackdriver-reverse-proxy.svg?branch=master)](https://travis-ci.org/GoogleCloudPlatform/stackdriver-reverse-proxy)

stackdriver-reverse-proxy is an HTTP/HTTPS proxy to automatically trace
all the incoming requests.

## Installation and usage

```
$ go get github.com/GoogleCloudPlatform/stackdriver-reverse-proxy/cmd/stackdriver-reverse-proxy
```

Once installed, start the proxy server and target your application server. For example, if
the application server is running on http://service:8080, the following command
will start the proxy at http://localhost:5555:

```
$ stackdriver-reverse-proxy -project=bamboo-lua-400 -target=http://service:8080 -http=:5555
```

The authentication is automatically handled if you are running the proxy server
on Google Cloud Platform. If not, see the [Application Default Credentials](https://developers.google.com/identity/protocols/application-default-credentials) guide to enable ADC.

## Overview

![Overview](http://i.imgur.com/Hsq4OcR.png)

This reverse proxy works as a sidecar. All the incoming requests will be proxied
and delievered to any of your app servers, specified with the -target flag.

The proxy provides:

- Down sampling and creation of trace headers for the incoming requests.
- Reporting of latencies to the [Stackdriver Trace](https://cloud.google.com/trace/).
- Soon: Reporting of metrics such as number of in-flight requests, success rate, request/response size.
- Soon: Reporting of reporting to the [Stackdriver Error Reporting](https://cloud.google.com/error-reporting/).


## Disclaimer

This is not an official Google product and it is in active development.

## License

Copyright 2017 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
