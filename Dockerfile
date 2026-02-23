FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/wolfbbs ./cmd/wolfbbs
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/wolfbbs-web ./cmd/wolfbbs-web
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/wolfbbs-irc ./cmd/wolfbbs-irc
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/wolfbbs-mailin ./cmd/wolfbbs-mailin
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/wolfbbs-trivia ./cmd/doors-trivia

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/wolfbbs /app/wolfbbs
COPY --from=build /out/wolfbbs-web /app/wolfbbs-web
COPY --from=build /out/wolfbbs-irc /app/wolfbbs-irc
COPY --from=build /out/wolfbbs-mailin /app/wolfbbs-mailin
COPY --from=build /out/wolfbbs-trivia /app/wolfbbs-trivia
EXPOSE 2222 8080 6667 8091
ENTRYPOINT ["/app/wolfbbs", "-listen", ":2222"]
