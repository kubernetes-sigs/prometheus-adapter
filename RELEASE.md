# Release Process

prometheus-adapter is released on an as-needed basis. The process is as follows:

1. An issue is proposing a new release with a changelog since the last release
1. At least one [OWNERS](OWNERS) must LGTM this release
1. A PR that bumps version hardcoded in code is created and merged
1. An OWNER creates a draft Github release
1. An OWNER creates a release tag using `git tag -s $VERSION`, inserts the changelog and pushes the tag with `git push $VERSION`. Then waits for [prow.k8s.io](https://prow.k8s.io) to build and push new images to [gcr.io/k8s-staging-prometheus-adapter](https://gcr.io/k8s-staging-prometheus-adapter)
1. A PR in [kubernetes/k8s.io](https://github.com/kubernetes/k8s.io/blob/main/k8s.gcr.io/images/k8s-staging-prometheus-adapter/images.yaml) is created to release images to `k8s.gcr.io`
1. An OWNER publishes the GitHub release
1. An announcement email is sent to `kubernetes-sig-instrumentation@googlegroups.com` with the subject `[ANNOUNCE] prometheus-adapter $VERSION is released`
1. The release issue is closed
