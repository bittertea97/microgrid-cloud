# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.25.6

FROM golang:${GO_VERSION}-alpine AS builder

RUN apk add --no-cache ca-certificates git
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/microgrid-cloud ./

FROM gcr.io/distroless/base-debian12 AS runtime

WORKDIR /app
COPY --from=builder /out/microgrid-cloud /app/microgrid-cloud

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/microgrid-cloud"]