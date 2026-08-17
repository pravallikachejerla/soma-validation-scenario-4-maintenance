# Multi-stage build for the Go API binary.
FROM golang:1.22-alpine AS base
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download

FROM base AS api
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM base AS seed
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

FROM base AS migrate
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=api /out/api /app/api
COPY --from=seed /out/seed /app/seed
COPY --from=migrate /out/migrate /app/migrate
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]

FROM gcr.io/distroless/static-debian12:nonroot AS seed-image
COPY --from=seed /out/seed /app/seed
USER nonroot:nonroot
ENTRYPOINT ["/app/seed"]

FROM gcr.io/distroless/static-debian12:nonroot AS migrate-image
COPY --from=migrate /out/migrate /app/migrate
USER nonroot:nonroot
ENTRYPOINT ["/app/migrate"]
