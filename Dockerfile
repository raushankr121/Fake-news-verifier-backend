FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main .

# 2. Run the App in a container with Chrome installed
FROM alpine:latest
RUN apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont

# Tell the app where Chrome is
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/

WORKDIR /root/
COPY --from=builder /app/main .

EXPOSE 8080
CMD ["./main"]