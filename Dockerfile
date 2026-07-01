FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/dashboard ./cmd/dashboard && \
    CGO_ENABLED=0 go build -trimpath -o /out/conductor ./cmd/conductor && \
    CGO_ENABLED=0 go build -trimpath -o /out/h2-thrasher ./cmd/h2-thrasher && \
    CGO_ENABLED=0 go build -trimpath -o /out/l7-abuser ./cmd/l7-abuser && \
    CGO_ENABLED=0 go build -trimpath -o /out/quic-burner ./cmd/quic-burner && \
    CGO_ENABLED=0 go build -trimpath -o /out/slowloris ./cmd/slowloris && \
    CGO_ENABLED=0 go build -trimpath -o /out/ws-flood ./cmd/ws-flood && \
    CGO_ENABLED=0 go build -trimpath -o /out/sync-runtime ./cmd/sync-runtime && \
    CGO_ENABLED=0 go build -trimpath -o /out/vector-bench ./cmd/vector-bench

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/* /app/
EXPOSE 8089