ARG GO_VERSION

FROM golang:${GO_VERSION} as build

WORKDIR /go/src/sigs.k8s.io/prometheus-adapter
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY pkg pkg
COPY cmd cmd
COPY Makefile Makefile

ARG ARCH
RUN make prometheus-adapter

FROM gcr.io/distroless/static:latest

COPY --from=build /go/src/sigs.k8s.io/prometheus-adapter/adapter /
USER 65534
ENTRYPOINT ["/adapter"]
