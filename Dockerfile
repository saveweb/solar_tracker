FROM golang:latest AS builder
COPY . /src

WORKDIR /src

RUN CGO_ENABLED=0 go build -ldflags="-extldflags=-static -s -w" -o solar_tracker

FROM alpine:latest

COPY --from=builder /src/solar_tracker /app/solar_tracker

RUN chmod +x /app/solar_tracker

WORKDIR /app

CMD ["/app/solar_tracker"]
