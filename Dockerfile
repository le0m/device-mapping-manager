###
# Build binary
###
FROM golang:1.25 AS builder

ARG VERSION=development
ENV DEBIAN_FRONTEND=noninteractive

WORKDIR /src

COPY go.mod go.sum /src/
RUN go mod download

COPY *.go /src/
COPY internal /src/internal
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags "-s -X main.Version=${VERSION} -linkmode external -extldflags -static" -o /dmm

###
# Final image
###
FROM alpine:3

WORKDIR /

COPY --from=builder /dmm /dmm

ENTRYPOINT ["/dmm"]
