# Ares
Phishing toolkit for red teams and pentesters.

Ares allows security testers to create a phishing environment easily. Ares acts as a proxy between the original site and the phished site, and allows modifications and injects. 

## Features

* transparant 1 to 1 of original site
* modify specific paths to return other pages
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

```
go get github.com/dutchcoders/ares/cmd
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

#socks = "socks4://127.0.0.1:9050"

[[host]]
host = "test.lvh.me"
target = "http://www.nu.nl/"

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
