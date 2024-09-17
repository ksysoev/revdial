# RevDial

[![RevDial CI](https://github.com/ksysoev/revdial/actions/workflows/main.yml/badge.svg)](https://github.com/ksysoev/revdial/actions/workflows/main.yml)
[![codecov](https://codecov.io/gh/ksysoev/revdial/graph/badge.svg?token=M0J6DL8OAU)](https://codecov.io/gh/ksysoev/revdial)
[![Go Report Card](https://goreportcard.com/badge/github.com/ksysoev/revdial)](https://goreportcard.com/report/github.com/ksysoev/revdial)
[![Go Reference](https://pkg.go.dev/badge/github.com/ksysoev/revdial.svg)](https://pkg.go.dev/github.com/ksysoev/revdial)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)



## Overview

RevDial is a Go package that provides a mechanism for managing network connections through reverse dialing. It allows an accepted connection to be transformed into a dialer. This dialer can then create new network connections back to the original dialer, which in turn uses a listener to accept these connections. This functionality is particularly useful in scenarios like exposing a service that is hidden behind a NAT, where you want to proxy requests via a publicly exposed server.

This package is inspired by the [Go revdial/v2 package](https://pkg.go.dev/golang.org/x/build@v0.0.0-20240917013954-30938eb34c8a/revdial/v2), which focuses on reverse dialing HTTP services. Unlike that package, RevDial is not limited to the HTTP protocol and can handle any TCP-based protocols.

## Installation


To install the package, use the following command:

```sh
go get github.com/ksysoev/revdial
```

## Usage

### Using a Dialer

```go
package main

import (
	"context"
	"fmt"
	"github.com/ksysoev/revdial"
)

func main() {
	dialer := revdial.NewDialer("localhost:9090")

	if err := dialer.Start(context.Background()); err != nil {
		fmt.Printf("Failed to start dialer: %v\n", err)
		return
	}

	defer dialer.Stop()

	conn, err := dialer.DialContext(ctx)
	if err != nil {
		fmt.Printf("failed to dial: %v", err)
		return
	}

    defer conn.Close()

    // Here your logic for handling connection
}
```

### Using a Listener

```go
package main

import (
	"context"
	"fmt"
	"github.com/ksysoev/revdial"
)

func main() {
	listener, err := revdial.Listen(context.Background(), "localhost:9090")
	if err != nil {
		fmt.Printf("Failed to create listener: %v\n", err)
		return
	}
	defer listener.Close()

	fmt.Println("Listener created")

    for  {
        conn, err := listener.Accept()
        if err != nil {
            fmt.Printf("Failed to accept connection: %v\n", err)
		    return
        }
    
        // Here logic for handling incomming connection

        conn.Close()
    }
}
```


## Planned Features

- **Authentication Support**: Add support for various authentication mechanisms to secure connections.
- **TLS Support**: Implement TLS support to encrypt the data transmitted over the connections.
- **Configuration Options**: Provide more configuration options to customize the behavior of the dialer and listener.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request with your changes. Make sure to follow the project's coding guidelines and include tests for any new features or bug fixes.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
