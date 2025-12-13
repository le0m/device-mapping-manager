# syntax=docker/dockerfile:1

FROM golang:1.25

ARG VERSION=development
ENV DEBIAN_FRONTEND noninteractive

WORKDIR /go/src/github.com/allfro/device-volume-driver

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -ldflags "-s -X main.Version=${VERSION} -linkmode external -extldflags -static" -o /dvd

FROM alpine

WORKDIR /

COPY --from=0 /dvd /dvd

ENTRYPOINT ["/dvd"]
