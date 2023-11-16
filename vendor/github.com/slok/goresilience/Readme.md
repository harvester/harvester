# Goresilience [![Build Status][travis-image]][travis-url] [![Go Report Card][goreport-image]][goreport-url] [![GoDoc][godoc-image]][godoc-url]

Goresilience is a Go toolkit to increase the resilience of applications. Inspired by hystrix and similar libraries at it's core but at the same time very different:

## Features

- Increase resilience of the programs.
- Easy to extend, test and with clean design.
- Go idiomatic.
- Use the decorator pattern (middleware), like Go's http.Handler does.
- Ability to create custom resilience flows, simple, advanced, specific... by combining different runners in chains.
- Safety defaults.
- Not couple to any framework/library.
- Prometheus/Openmetrics metrics as first class citizen.

## Table of Contents

- [Motivation](#motivation)
- [Getting started](#getting-started)
- [Static Runners](#static-runners)
  - [Timeout](#timeout)
  - [Retry](#retry)
  - [Bulkhead](#bulkhead)
  - [Circuit breaker](#circuit-breaker)
  - [Chaos](#chaos)
- [Adaptive Runners](#adaptive-runners)
  - [Concurrency limit](#concurrency-limit)
    - [Executors](#executors)
    - [Limiter](#limiter)
    - [Result policy](#result-policy)
- [Other](#other)
  - [Metrics](#metrics)
  - [Hystrix-like](#hystrix-like)
  - [HTTP-middleware](#http-middleware)
- [Architecture](#architecture)
- [Extend using your own runners](#extend-using-your-own-runners)

## Motivation

You are wondering, why another circuit breaker library...?

Well, this is not a circuit breaker library. Is true that Go has some good circuit breaker libraries (like [sony/gobreaker], [afex/hystrix-go] or [rubyist/circuitbreaker]). But there is a lack a resilience toolkit that is easy to extend, customize and establishes a design that can be extended, that's why goresilience born.

The aim of goresilience is to use the library with the resilience runners that can be combined or used independently depending on the execution logic nature (complex, simple, performance required, very reliable...).

Also one of the key parts of goresilience is the extension to create new runners yourself and use it in combination with the bulkhead, the circuitbreaker or any of the runners of this library or from others.

## Getting started

The usage of the library is simple. Everything is based on `Runner` interface.

The runners can be used in two ways, in standalone mode (one runner):

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/slok/goresilience/timeout"
)

func main() {
    // Create our command.
    cmd := timeout.New(timeout.Config{
        Timeout: 100 * time.Millisecond,
    })

    for i := 0; i < 200; i++ {
        // Execute.
        result := ""
        err := cmd.Run(context.TODO(), func(_ context.Context) error {
            if time.Now().Nanosecond()%2 == 0 {
                time.Sleep(5 * time.Second)
            }
            result = "all ok"
            return nil
        })

        if err != nil {
            result = "not ok, but fallback"
        }

        log.Printf("the result is: %s", result)
    }
}

```

or combining in a chain of multiple runners by combining runner middlewares. In this example the execution will be retried timeout and concurrency controlled using a runner chain:

```go
package main

import (
    "context"
    "errors"
    "fmt"

    "github.com/slok/goresilience"
    "github.com/slok/goresilience/bulkhead"
    "github.com/slok/goresilience/retry"
    "github.com/slok/goresilience/timeout"
)

func main() {
    // Create our execution chain.
    cmd := goresilience.RunnerChain(
        bulkhead.NewMiddleware(bulkhead.Config{}),
        retry.NewMiddleware(retry.Config{}),
        timeout.NewMiddleware(timeout.Config{}),
    )

    // Execute.
    calledCounter := 0
    result := ""
    err := cmd.Run(context.TODO(), func(_ context.Context) error {
        calledCounter++
        if calledCounter%2 == 0 {
            return errors.New("you didn't expect this error")
        }
        result = "all ok"
        return nil
    })

    if err != nil {
        result = "not ok, but fallback"
    }

    fmt.Printf("result: %s", result)
}
```

As you see, you could create any combination of resilient execution flows by combining the different runners of the toolkit.

## Static Runners

Static runners are the ones that based on a static configuration and don't change based on the environment (unlike the adaptive ones).

### Timeout

This runner is based on timeout pattern, it will execute the `goresilience.Func` but if the execution duration is greater than a T duration timeout it will return a timeout error.

Check [example][timeout-example].

### Retry

This runner is based on retry pattern, it will retry the execution of `goresilience.Func` in case it failed N times.

It will use a exponential backoff with some jitter (for more information check [this][amazon-retry])

Check [example][retry-example].

### Bulkhead

This runner is based on [bulkhead pattern][bulkhead-pattern], it will control the concurrency of `goresilience.Func` executions using the same runner.

It also can timeout if a `goresilience.Func` has been waiting too much to be executed on a queue of execution.

Check [example][bulkhead-example].

### Circuit breaker

This runner is based on [circuitbreaker pattern][circuit-breaker-url], it will be storing the results of the executed `goresilience.Func` in N buckets of T time to change the state of the circuit based on those measured metrics.

Check [example][circuitbreaker-example].

### Chaos

This runner is based on [failure injection][chaos-engineering] of errors and latency. It will inject those failures on the required executions (based on percent or all).

Check [example][chaos-example].

## Adaptive Runners

### Concurrency limit

Concurrency limit is based on Netflix [concurrency-limit] library. It tries to implement the same features but for goresilience library (nd compatible with other runners).

It limits the concurrency based on less configuration and adaptive based on the environment is running on that moment, hardware, load...

This Runner will limit the concurrency (like bulkhead) but it will use different TCP congestion algorithms to adapt the concurrency limit based on errors and latency.

The Runner is based on 4 components.

- Limiter: This is the one that will measure and calculate the limit of concurrency based on different algorithms that can be choose, for example [AIMD].
- Executor: This is the one executing the `goresilience.Func` itself, it has different queuing implementations that will prioritize and drop executions based on the implementations.
- Runner: This is the runner itself that will be used by the user and is the glue of the `Limiter` and the `Executor`. This will had a policy that will treat the execution result as an error, success or ignore for the Limiter algorithm.
- Result policy: This is a function that can be configured on the concurrencylimit Runner. This function receives the result of the executed function and returns a result for the limit algorithm. This policy is responsible to tell the limit algorithm if the received error should be count as a success, failure or ignore on the calculation of the concurrency limit. For example: only count the errors that have been 502 other ones ignore.

Check [AIMD example][concurrencylimit-example].
Check [CoDel example][codel-example].

#### Executors

- `FIFO`: This executor is the default one it will execute the queue jobs in a first-in-first-out order and also has a queue wait timeout.
- `LIFO`: This executor will execute the queue jobs in a last-in-first-out order and also has a queue wait timeout.
- `AdaptiveLIFOCodel`: Implementation of Facebook's [CoDel+adaptive LIFO][fb-codel] algorithm. This executor is used with `Static` limiter.

#### Limiter

- `Static`: This limiter will set a constant limit that will not change.
- `AIMD`: This limiter is based on [AIMD] TCP congestion algorithm. It increases the limit at a constant rate and when congestion occurs (by timeout or result failure) it will decrease by a configured factor

#### Result policy

- `everyExternalErrorAsFailurePolicy`: is the default policy. for errors that are `errors.ErrRejectedExecution` they will act as ignored by the limit algorithms, the rest of the errors will be treat as failures.

## Other

### Metrics

All the runners can be measured using a `metrics.Recorder`, but instead of passing to every runner, the runners will try to get this recorder from the context. So you can wrap any runner using `metrics.NewMiddleware` and it will activate the metrics support on the wrapped runners. This should be the first runner of the chain.

At this moment only [Prometheus][prometheus-url] is supported.

In this [example][hystrix-example] the runners are measured.

Measuring has always a performance hit (not too high), on most cases is not a problem, but there is a benchmark to see what are the numbers:

```text
BenchmarkMeasuredRunner/Without_measurement_(Dummy).-4            300000              6580 ns/op             677 B/op         12 allocs/op
BenchmarkMeasuredRunner/With_prometheus_measurement.-4            200000             12901 ns/op             752 B/op         15 allocs/op
```

### Hystrix-like

Using the different runners a hystrix like library flow can be obtained. You can see a simple example of how it can be done on this [example][hystrix-example]

### http middleware

Creating HTTP middlewares with goresilience runners is simple and clean. You can see an example of how it can be done on this [example][http-example]. The example shows how you can protect the server by load shedding using an adaptive concurrencylimit `goresilience.Runner`.

## Architecture

At its core, goresilience is based on a very simple idea, the `Runner` interface, `Runner` interface is the unit of execution, its accepts a `context.Context`, a `goresilience.Func` and returns an `error`.

The idea of the Runner is the same as the go's `http.Handler`, having a interface you could create chains of runners, also known as middlewares (Also called decorator pattern).

The library comes with decorators called `Middleware` that return a function that wraps a runner with another runner and gives us the ability to create a resilient execution flow having the ability to wrap any runner to customize with the pieces that we want including custom ones not in this library.

This way we could create execution flow like this example:

```text
Circuit breaker
└── Timeout
    └── Retry
```

## Extend using your own runners

To create your own runner, You need to have 2 things in mind.

- Implement the `goresilience.Runner` interface.
- Give constructors to get a `goresilience.Middleware`, this way your `Runner` could be chained with other `Runner`s.

In this example (full example [here][extend-example]) we create a new resilience runner to make chaos engineering that will fail at a constant rate set on the `Config.FailEveryTimes` setting.

Following the library convention with `NewFailer` we get the standalone Runner (the one that is not chainable). And with `NewFailerMiddleware` We get a `Middleware` that can be used with `goresilience.RunnerChain` to chain with other Runners.

Note: We can use `nil` on `New` because `NewMiddleware` uses `goresilience.SanitizeRunner` that will return a valid Runner as the last part of the chain in case of being `nil` (for more information about this check `goresilience.command`).

```golang
// Config is the configuration of constFailer
type Config struct {
    // FailEveryTimes will make the runner return an error every N executed times.
    FailEveryTimes int
}

// New is like NewFailerMiddleware but will not wrap any other runner, is standalone.
func New(cfg Config) goresilience.Runner {
    return NewFailerMiddleware(cfg)(nil)
}

// NewMiddleware returns a new middleware that will wrap runners and will fail
// every N times of executions.
func NewMiddleware(cfg Config) goresilience.Middleware {
    return func(next goresilience.Runner) goresilience.Runner {
        calledTimes := 0
        // Use the RunnerFunc helper so we don't need to create a new type.
        return goresilience.RunnerFunc(func(ctx context.Context, f goresilience.Func) error {
            // We should lock the counter writes, not made because this is an example.
            calledTimes++

            if calledTimes == cfg.FailEveryTimes {
                calledTimes = 0
                return fmt.Errorf("failed due to %d call", calledTimes)
            }

            // Run using the the chain.
            next = runnerutils.Sanitize(next)
            return next.Run(ctx, f)
        })
    }
}
```

[travis-image]: https://travis-ci.org/slok/goresilience.svg?branch=master
[travis-url]: https://travis-ci.org/slok/goresilience
[goreport-image]: https://goreportcard.com/badge/github.com/slok/goresilience
[goreport-url]: https://goreportcard.com/report/github.com/slok/goresilience
[godoc-image]: https://godoc.org/github.com/slok/goresilience?status.svg
[godoc-url]: https://godoc.org/github.com/slok/goresilience
[sony/gobreaker]: https://github.com/sony/gobreaker
[afex/hystrix-go]: https://github.com/afex/hystrix-go
[rubyist/circuitbreaker]: https://github.com/rubyist/circuitbreaker
[circuit-breaker-url]: https://martinfowler.com/bliki/CircuitBreaker.html
[retry-example]: examples/retry
[timeout-example]: examples/timeout
[bulkhead-example]: examples/bulkhead
[circuitbreaker-example]: examples/circuitbreaker
[chaos-example]: examples/chaos
[hystrix-example]: examples/hystrix
[http-example]: examples/http
[extend-example]: examples/extend
[concurrencylimit-example]: examples/concurrencylimit
[codel-example]: examples/codel
[amazon-retry]: https://aws.amazon.com/es/blogs/architecture/exponential-backoff-and-jitter/
[bulkhead-pattern]: https://docs.microsoft.com/en-us/azure/architecture/patterns/bulkhead
[chaos-engineering]: https://en.wikipedia.org/wiki/Chaos_engineering
[prometheus-url]: http://prometheus.io
[aimd]: https://en.wikipedia.org/wiki/Additive_increase/multiplicative_decrease
[concurrency-limit]: https://github.com/Netflix/concurrency-limits
[aimd]: https://en.wikipedia.org/wiki/Additive_increase/multiplicative_decrease
[fb-codel]: https://queue.acm.org/detail.cfm?id=2839461
