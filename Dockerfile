# Use the offical Golang image to create a build artifact.
# This is based on Debian and sets the GOPATH to /go.
# https://hub.docker.com/_/golang
FROM golang:1.19-alpine as builder
RUN apk add --no-cache gcc libc-dev git

ARG version=develop

# Copy local code to the container image.
WORKDIR /go/src/github.com/Mouhamadou305/splunk-sli-provider

# Force the go compiler to use modules
ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org
ENV BUILDFLAGS=""

# Copy `go.mod` for definitions and `go.sum` to invalidate the next layer
# in case of a change in the dependencies
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy local code to the container image.
COPY . .

# `skaffold debug` sets SKAFFOLD_GO_GCFLAGS to disable compiler optimizations
ARG SKAFFOLD_GO_GCFLAGS

# Build the command inside the container.
# (You may fetch or manage dependencies here, either manually or with a tool like "godep".)
# RUN GOOS=linux go build -ldflags '-linkmode=external' -gcflags="${SKAFFOLD_GO_GCFLAGS}" -v -o splunk-sli-provider
RUN go build -o splunk-sli-provider

# Use a Docker multi-stage build to create a lean production image.
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
FROM alpine:3.16
# Install extra packages
# See https://github.com/gliderlabs/docker-alpine/issues/136#issuecomment-272703023

RUN    apk update && apk upgrade \
	&& apk add ca-certificates libc6-compat \
	&& update-ca-certificates \
	&& rm -rf /var/cache/apk/*

ARG version
ENV version $version

ENV env=production
ARG debugBuild

# Install python/pip
ENV PYTHONUNBUFFERED=1
RUN apk add --update --no-cache python3 && ln -sf python3 /usr/bin/python
RUN python3 -m ensurepip
RUN pip3 install --no-cache --upgrade pip setuptools

COPY splunk.py /
COPY requirements.txt /

RUN pip install --no-cache-dir -r requirements.txt

# Copy the binary to the production image from the builder stage.
COPY --from=builder /go/src/github.com/Mouhamadou305/splunk-sli-provider/splunk-sli-provider /splunk-sli-provider

# required for external tools to detect this as a go binary
ENV GOTRACEBACK=all

# Run the web service on container startup.
CMD ["/splunk-sli-provider"]
