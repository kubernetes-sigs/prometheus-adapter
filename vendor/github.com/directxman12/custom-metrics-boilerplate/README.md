# Custom Metrics Adpater Server Boilerplate

## Purpose

This repository contains boilerplate code for setting up an implementation
of the custom metrics API (https://github.com/kubernetes/metrics).

It includes the necessary boilerplate for setting up an implementation
(generic API server setup, registration of resources, etc), plus a sample
implementation backed by fake data.

## How to use this repository

In order to use this repository, you should vendor this repository at
`github.com/directxman12/custom-metrics-boilerplate`, and implement the
`"github.com/directxman12/custom-metrics-boilerplate/pkg/provider".CustomMetricsProvider`
interface.  You can then pass this to the main setup functions.

The `pkg/cmd` package contains the building blocks of the actual API
server setup.  You'll most likely want to wrap the existing options and
flags setup to add your own flags for configuring your provider.

A sample implementation of this can be found in the file `sample-main.go`
and `pkg/sample-cmd` directory.  You'll want to have the equivalent files
in your project.

### A note on Dependencies

You'll need to `glide install` dependencies before you can use this
project.

## Compatibility

The APIs in this repository follow the standard guarantees for Kubernetes
APIs, and will follow Kubernetes releases.

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community
page](http://kubernetes.io/community/).

You can reach the maintainers of this repository at:

- Slack: #sig-instrumention (on https://kubernetes.slack.com -- get an
  invite at slack.kubernetes.io)
- Mailing List:
  https://groups.google.com/forum/#!forum/kubernetes-sig-instrumentation

### Code of Conduct

Participation in the Kubernetes community is governed by the [Kubernetes
Code of Conduct](code-of-conduct.md).

### Contibution Guidelines

See [CONTRIBUTING.md](CONTRIBUTING.md) for more information.
