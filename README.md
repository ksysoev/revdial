# RevDial

[![RevDial CI](https://github.com/ksysoev/revdial/actions/workflows/main.yml/badge.svg)](https://github.com/ksysoev/revdial/actions/workflows/main.yml)
[![codecov](https://codecov.io/gh/ksysoev/revdial/graph/badge.svg?token=M0J6DL8OAU)](https://codecov.io/gh/ksysoev/revdial)
[![Go Report Card](https://goreportcard.com/badge/github.com/ksysoev/revdial)](https://goreportcard.com/report/github.com/ksysoev/revdial)
[![Go Reference](https://pkg.go.dev/badge/github.com/ksysoev/revdial.svg)](https://pkg.go.dev/github.com/ksysoev/revdial)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)



Package revdial provides a Dialer and Listener that collaborate to transform an accepted connection into a reverse Dialer. It enables the creation of net.Conns that connect back to the original dialer, facilitating a net.Listener to accept those connections.
