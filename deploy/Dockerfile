FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sonichub ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/sonichub /app/sonichub
COPY migrations /app/migrations
RUN mkdir -p /app/data/media/generated && chown -R app:app /app
USER app
EXPOSE 8080
ENV HTTP_ADDR=:8080 MEDIA_DIR=/app/data/media MIGRATIONS_DIR=/app/migrations
CMD ["/app/sonichub"]
