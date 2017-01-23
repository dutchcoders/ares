# Ares [![Gitter](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/dutchcoders/ares?utm_source=badge&utm_medium=badge&utm_campaign=&utm_campaign=pr-badge&utm_content=badge) [![Go Report Card](https://goreportcard.com/badge/dutchcoders/ares)](https://goreportcard.com/report/dutchcoders/ares) [![Docker pulls](https://img.shields.io/docker/pulls/dutchsec/ares.svg)](https://hub.docker.com/r/dutchsec/ares/) [![Build Status](https://travis-ci.org/dutchcoders/ares.svg?branch=master)](https://travis-ci.org/dutchcoders/ares)

Phishing toolkit for red teams and pentesters. Ares allows security testers to create a phishing environment easily, based on real sites. Ares acts as a proxy between the phised and original site, and allows (realtime) modifications and injects. 

# Getting started

## Docker

Make sure the config toml is located and valid. 

```
docker run -d -p 8080:8080 --name ares -v $(pwd)/config.toml:/etc/ares.toml dutchsec/ares
```

navigate to http://wikipedia.lvh.me:8080/


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

## Installation

Make sure you have Go 1.7 installed. 

```
git clone git@github.com:dutchcoders/ares.git
go run main.go -c config.toml
```

## Injects

The injects can be inserted in the target site, currently we have the following injects:

* **location** will ask the client for longitude and latitude and post to server
* **snap** will generate screenshots and post to server
* **clipboard** will copy text from clipboard and post to server

## Configuration

See config.toml.sample for a sample configuration file.

## Gophish

Ares will work seamless with Gophish, where you'll use Ares for the landing page functionality. 
