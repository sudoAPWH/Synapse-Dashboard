FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN go build -o dashboard .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=build /app/dashboard /usr/local/bin/dashboard
EXPOSE 3000
CMD ["dashboard"]
