FROM --platform=$BUILDPLATFORM golang:1.26.1-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs ./cmd/wolfbbs
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-web ./cmd/wolfbbs-web
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-irc ./cmd/wolfbbs-irc
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-mailin ./cmd/wolfbbs-mailin
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-trivia ./cmd/doors-trivia
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/oputil ./cmd/oputil

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl docker-cli docker-cli-compose netcat-openbsd
LABEL org.opencontainers.image.title="WolfBBS" \
      org.opencontainers.image.description="SSH-first bulletin board system with web companion, IRC bridge, doors, and turnkey installation" \
      org.opencontainers.image.source="https://github.com/Awassee/wolfbbs" \
      org.opencontainers.image.licenses="MIT"
WORKDIR /app
COPY --from=build /out/wolfbbs /app/wolfbbs
COPY --from=build /out/wolfbbs-web /app/wolfbbs-web
COPY --from=build /out/wolfbbs-irc /app/wolfbbs-irc
COPY --from=build /out/wolfbbs-mailin /app/wolfbbs-mailin
COPY --from=build /out/wolfbbs-trivia /app/wolfbbs-trivia
COPY --from=build /out/oputil /app/oputil
RUN chmod +x /app/wolfbbs /app/wolfbbs-web /app/wolfbbs-irc /app/wolfbbs-mailin /app/wolfbbs-trivia /app/oputil
EXPOSE 2222 8080 6667 8091
ENTRYPOINT ["/app/wolfbbs", "-listen", ":2222"]
