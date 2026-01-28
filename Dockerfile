# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.22

FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache ca-certificates git
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/microgrid-cloud ./

FROM alpine:3.20 AS runtime

WORKDIR /app
COPY --from=builder /out/microgrid-cloud /app/microgrid-cloud
RUN apk add --no-cache ca-certificates

EXPOSE 8080

ENTRYPOINT ["/app/microgrid-cloud"]
