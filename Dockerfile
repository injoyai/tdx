FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tdx-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
COPY --from=builder /tdx-server /usr/local/bin/tdx-server
EXPOSE 8001
ENTRYPOINT ["tdx-server"]
