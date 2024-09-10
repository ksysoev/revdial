# RevDial

[![RevDial CI](https://github.com/ksysoev/revdial/actions/workflows/main.yml/badge.svg)](https://github.com/ksysoev/revdial/actions/workflows/main.yml)
[![codecov](https://codecov.io/gh/ksysoev/revdial/graph/badge.svg?token=M0J6DL8OAU)](https://codecov.io/gh/ksysoev/revdial)


Package revdial provides a Dialer and Listener that collaborate to transform an accepted connection into a reverse Dialer. It enables the creation of net.Conns that connect back to the original dialer, facilitating a net.Listener to accept those connections.
