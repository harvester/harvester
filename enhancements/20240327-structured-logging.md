# Structured Logging

## Summary

Harvester should emit only structured log messages. Existing non-structured log
messages should be converted to structured log messages.

### Related Issues

- https://github.com/harvester/harvester/issues/5402

## Motivation

Unstructured logging is nice to read for humans, but the lack of structure makes
it hard to apply tooling to it. This in turn makes it hard to manage large
amounts of logs, apply automatic analysis and fault detection and filtering.
Therefore we should utilize structured logging whenever possible.
Log analysis is not only a concern for Harvester users, but also for Harvester
developers and support engineers, when they need to analyze support bundles.

### Goals

- All log messages emitted by Harvester should be structured
- The structure should utilize standardized field names
- Implementation should require a minimal amount of new dependencies
- A migration strategy for the existing code base to convert existing log
  messages to structured logging

### Non-Goals

This HEP does not cover:

- integration with log analysis tools
- log storage, access, viewing, retention policies
- when to log or what severity to log at

## Proposal

To implement this proposal, several steps should be taken. First, the developer
guide should receive an update, describing how developers should implement
logging. This includes the mention of which library to use for structured
logging and how to implement structured logging with this library, including any
standard fields and conventions.
Second, all existing log messages should be converted to structured logging.
Lastly, a CI check should be put in place to prevent new additions of
unstructured logging, e.g. using the tool [Logcheck](https://github.com/kubernetes-sigs/logtools/tree/main/logcheck)

### Logging infrastructure

Harvester utilizes the Golang library Logrus to emit log messages. This library
has builtin support for structured logging.

### Standard Structure Fields

It is vitally important to define a set of standard structure field names to be
used in structured logging to enable the automatic consumption of logs.
As an example, consider the following two hypothetical log messages with
non-standardized structure field names:

```
{"namespace": "default", "name": "test", "msg": "creating VM"}
{"k8s_ns": "default", "vm_display_name": test: "message":"could not initialize"}
```

While both messages contain the same three fields (a namespace, a name and a
message), the use of different field names for the three fields makes it
unnecessarily hard to e.g. create a filter matching both messages.
With the use of standardized field names throughout the log emitting program,
this problem does not arise:

```
{"namespace": "default", "name": "test", "msg": "creating VM"}
{"namespace": "default", "name": "test", "msg": "could not initialize"}
```

These are possible field names to use:

| Field Name | K8s Resource Field | Description |
| ---------- | ------------------ | ----------- |
| namespace | `.metadata.namespace` | K8s namespace of the subject of the message |
| name | `.metadata.name` | Name of the K8s resource which is the subject of the message |
| kind | `.kind` | Kind of the K8s resource which is subject of the log message |
| err | n/a | String representation of a `builtin.error` value |
| apiVersion | `.apiVersion` | K8s API-version and API-group information of the subject of the message |

### Migration Strategy

Existing code for emitting logs in Harvester often times looks something like
this:

```
logrus.Errorf("something went wrong with %s/%s", obj.Namespace, obj.Name)
```

This code can easily be converted into structured logging, utilizing Logrus'
builtin facilities:

```
logrus.WithFields(logrus.Fields{
  "namespace": obj.Namespace,
  "name": obj.Name,
}).Error("something went wrong")
```

### API Changes

None

## See Also

[KEP for structured logging in K8s](https://github.com/kubernetes/enhancements/blob/master/keps/sig-instrumentation/1602-structured-logging/README.md)
