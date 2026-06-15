# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY *.go ./
COPY web/ web/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /invtts .

# Runtime stage
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /invtts /invtts
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/invtts"]
