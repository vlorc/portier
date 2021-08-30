# [Golang portier](https://github.com/vlorc/portier)
[中文](README_CN.md)

[![License](https://img.shields.io/:license-apache-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![codebeat badge](https://codebeat.co/badges/c41b426c-4121-4dc8-99c2-f1b60574be64)](https://codebeat.co/projects/github-com-vlorc-portier-master)
[![Go Report Card](https://goreportcard.com/badge/github.com/vlorc/portier)](https://goreportcard.com/report/github.com/vlorc/portier)
[![Build Status](https://travis-ci.org/vlorc/portier.svg?branch=master)](https://travis-ci.org/vlorc/portier?branch=master)
[![Coverage Status](https://coveralls.io/repos/github/vlorc/portier/badge.svg?branch=master)](https://coveralls.io/github/vlorc/portier?branch=master)

Nginx cas! 200+ lines of small email authentication service

![login](img/login.png)

## Installing
	go install github.com/vlorc/portier@latest

## Quick Start

+ portier

```shell
./portier                                  \ 
  -cookie.domain .example.com              \
  -mail.addr smtp.exmail.qq.com:465        \ 
  -mail.user xxxx                          \ 
  -mail.pass xxxx
```

+ app

```shell
python -m http.server -b 127.0.0.1 8080
```

+ nginx

```shell
nginx -c example/nginx.conf
```

# Keyword

**nginx cas, mail verify, auth_request, portier**