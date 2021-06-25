# jobq

[![Drone Status](https://drone.honestbee.com/api/badges/honestbee/jobq/status.svg?branch=develop)](https://drone.honestbee.com/honestbee/jobq)
[![Maintainability](https://api.codeclimate.com/v1/badges/ee67437d8c14bc7cf736/maintainability)](https://codeclimate.com/repos/5c46845c5263880240004a38/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/ee67437d8c14bc7cf736/test_coverage)](https://codeclimate.com/repos/5c46845c5263880240004a38/test_coverage)


---

**jobq** is a generic worker pool library with 
1. dynamic adjust worker number
2. self-expiring job
3. easy exported metrics

## Install

```bash
go get github.com/honestbee/jobq
```

## Run The Test

```bash
# under the jobq repo
export GO111MODULE=on
go mod vendor
go test -v -race ./...
```

## Examples

### basic usage

```golang
package main

import (
	"context"
        "time"

        "github.com/honestbee/jobq"
)

func main() {
        dispatcher := jobq.NewWorkerDispatcher()
        defer dispatcher.Stop()

        job := func(ctx context.Context) (interface{}, error) {
                time.Sleep(200 *time.Millisecond)
                return "success", nil
        }

        tracker := dispatcher.QueueFunc(context.Background(), job)

        payload, err := tracker.Result()
        status := tracker.Status()

        fmt.Printf("complete=%t\n", status.Complete)
        fmt.Printf("success=%t\n", status.Success)
        if err != nil {
                fmt.Printf("err=%s\n", err.Error())
        } else {
                fmt.Printf("payload=%s\n", payload)
        }

        // Output:
        // complete=true
        // success=true
        // payload=success
}
```

more [examples](./example_test.go)

## Contribution

please feel free to do the pull request !!!
