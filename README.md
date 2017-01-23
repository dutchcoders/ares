# Ares [![Gitter](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/dutchcoders/ares?utm_source=badge&utm_medium=badge&utm_campaign=&utm_campaign=pr-badge&utm_content=badge) [![Go Report Card](https://goreportcard.com/badge/dutchcoders/ares)](https://goreportcard.com/report/dutchcoders/ares) [![Docker pulls](https://img.shields.io/docker/pulls/dutchsec/ares.svg)](https://hub.docker.com/r/dutchsec/ares/) [![Build Status](https://travis-ci.org/dutchcoders/ares.svg?branch=master)](https://travis-ci.org/dutchcoders/ares)

Phishing toolkit for red teams and pentesters. Ares allows security testers to create a landing page easily, embedded within the original site. Ares acts as a proxy between the phised and original site, and allows (realtime) modifications and injects. All references to the original site are being rewritten to the new site. Users will use the site like they'll normally do, but every step will be recorded of influenced. Ares will work perfect with dns poisoning as well.

## Getting started

### Docker

Make sure the config toml is at the right location and valid. 

```
docker run -d -p 8080:8080 --name ares -v $(pwd)/config.toml:/etc/ares.toml dutchsec/ares
```

Now you can navigate to http://wikipedia.lvh.me:8080/. If you want all results to be written to Elasticsearch, don't forget to setup the Elasticsearch cluster.

### Installation from Source

If you do not have a working Golang (1.7) environment setup please follow Golang Installation Guide.

```
$ git clone git@github.com:dutchcoders/ares.git
$ go run main.go -c config.toml
```

## Features

* realtime 1 to 1 of original site
* modify specific paths to return static (rendered as Go template) files
* create redirects (short urls)
* inject scripts into target site
* support ssl (using lets encrypt)
* multiple targets / hosts
* enhanced filtering on path, method, ip addresses and useragent
* all requests and responses are being logged into Elasticsearch
* all data is being stored for caching / retrieval

## Todo

* create small frontend for configuration, monitoring and dashboard
* send emails from toolkit

## Injects

The injects can be inserted in the target site, currently we have the following injects:

* **location** will ask the client for longitude and latitude and post to server
* **snap** will generate screenshots and post to server
* **clipboard** will copy text from clipboard and post to server

## Configuration

See config.toml.sample for a sample configuration file.

## Gophish

Ares will work seamless with Gophish, where you'll use Ares for the landing page functionality. 

## Contribute

Contributions are welcome.

### Setup your Ares Github Repository

Fork Ares upstream source repository to your own personal repository. Copy the URL for ares from your personal github repo (you will need it for the git clone command below).

```sh
$ mkdir -p $GOPATH/src/github.com/ares
$ cd $GOPATH/src/github.com/ares
$ git clone <paste saved URL for personal forked ares repo>
$ cd ares
```

###  Developer Guidelines
``Ares`` community welcomes your contribution. To make the process as seamless as possible, we ask for the following:
* Go ahead and fork the project and make your changes. We encourage pull requests to discuss code changes.
    - Fork it
    - Create your feature branch (git checkout -b my-new-feature)
    - Commit your changes (git commit -am 'Add some feature')
    - Push to the branch (git push origin my-new-feature)
    - Create new Pull Request

* If you have additional dependencies for ``Ares``, ``Ares`` manages its dependencies using [govendor](https://github.com/kardianos/govendor)
    - Run `go get foo/bar`
    - Edit your code to import foo/bar
    - Run `make pkg-add PKG=foo/bar` from top-level directory

* If you have dependencies for ``Ares`` which needs to be removed
    - Edit your code to not import foo/bar
    - Run `make pkg-remove PKG=foo/bar` from top-level directory

* When you're ready to create a pull request, be sure to:
    - Have test cases for the new code. If you have questions about how to do it, please ask in your pull request.
    - Run `make verifiers`
    - Squash your commits into a single commit. `git rebase -i`. It's okay to force update your pull request.
    - Make sure `go test -race ./...` and `go build` completes.

* Read [Effective Go](https://github.com/golang/go/wiki/CodeReviewComments) article from Golang project
    - `Ares` project is fully conformant with Golang style
    - if you happen to observe offending code, please feel free to send a pull request

## Creators

**Remco Verhoef (DutchSec)**
- <https://twitter.com/remco_verhoef>
- <https://twitter.com/dutchcoders>

## Copyright and license

Code and documentation copyright 2017 Remco Verhoef.

Code released under [the Apache license](LICENSE).

## Disclaimer

Here should come an appropriate disclaimer, no warranties and Ares shouldn't be used for malicious intent.

