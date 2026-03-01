<<<<<<< ours
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
ARG TARGETOS
ARG TARGETARCH
=======
FROM golang:1.22 AS build
>>>>>>> theirs
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
<<<<<<< ours
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs ./cmd/wolfbbs
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-web ./cmd/wolfbbs-web
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-irc ./cmd/wolfbbs-irc
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-mailin ./cmd/wolfbbs-mailin
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/wolfbbs-trivia ./cmd/doors-trivia
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/oputil ./cmd/oputil

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl netcat-openbsd
WORKDIR /app
COPY --from=build /out/wolfbbs /app/wolfbbs
COPY --from=build /out/wolfbbs-web /app/wolfbbs-web
COPY --from=build /out/wolfbbs-irc /app/wolfbbs-irc
COPY --from=build /out/wolfbbs-mailin /app/wolfbbs-mailin
COPY --from=build /out/wolfbbs-trivia /app/wolfbbs-trivia
COPY --from=build /out/oputil /app/oputil
RUN chmod +x /app/wolfbbs /app/wolfbbs-web /app/wolfbbs-irc /app/wolfbbs-mailin /app/wolfbbs-trivia /app/oputil
EXPOSE 2222 8080 6667 8091
=======
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/wolfbbs ./cmd/wolfbbs

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/wolfbbs /app/wolfbbs
EXPOSE 2222
>>>>>>> theirs
ENTRYPOINT ["/app/wolfbbs", "-listen", ":2222"]
