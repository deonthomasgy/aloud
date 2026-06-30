# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY web/ web/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /invtts .

# Runtime stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /invtts /invtts
EXPOSE 8080
ENV PORT=8080
ENV DATA_DIR=/data
VOLUME ["/data"]
ENTRYPOINT ["/invtts"]
