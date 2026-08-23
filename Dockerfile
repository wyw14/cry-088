FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/worklog ./cmd/server
FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app && mkdir -p /app/var/uploads && chown -R app:app /app
WORKDIR /app
COPY --from=build /out/worklog /app/worklog
COPY api /app/api
USER app
EXPOSE 8080
ENTRYPOINT ["/app/worklog"]
