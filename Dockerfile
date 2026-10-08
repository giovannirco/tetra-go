# syntax=docker/dockerfile:1
# Two stages: compile a static binary, then ship it alone on distroless.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w \
        -X github.com/giovannirco/tetra-go/internal/version.Version=${VERSION} \
        -X github.com/giovannirco/tetra-go/internal/version.Commit=${COMMIT} \
        -X github.com/giovannirco/tetra-go/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/tetra ./cmd/tetra

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/giovannirco/tetra-go" \
      org.opencontainers.image.description="Tetra arithmetic API (Go)" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/tetra /tetra
USER nonroot:nonroot
EXPOSE 8000
HEALTHCHECK --interval=10s --timeout=3s --start-period=3s --retries=3 CMD ["/tetra", "healthcheck"]
ENTRYPOINT ["/tetra"]
