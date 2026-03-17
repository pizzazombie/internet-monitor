FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/internet-monitor ./cmd/internet-monitor

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=build /out/internet-monitor /usr/local/bin/internet-monitor

EXPOSE 5555
VOLUME ["/data"]

ENV IM_DATA_DIR=/data
ENV IM_LISTEN_ADDR=:5555

ENTRYPOINT ["/usr/local/bin/internet-monitor"]
