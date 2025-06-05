# FROM crrbtest001.azurecr.io/base/golang-latest-devc as builder
FROM golang:latest as builder
WORKDIR /app

# Copy .netrc for authentication
# COPY .netrc /root/.netrc


COPY ./go.mod .
COPY ./go.sum .
RUN go mod download

COPY ./src ./src
RUN pwd
RUN ls -la 
RUN go run ./src/generateswagger/generateswagger.go
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o jnq ./src/main.go

# ARG PROXY=
# ENV http_proxy=$PROXY
# ENV https_proxy=$PROXY

# RUN apk add --no-check-certificate ca-certificates openssl
# RUN openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes -subj "/C=SE/CN=www.example.com"

# FROM crrbtest001.azurecr.io/base/alpine/alpine-3:3
from debian:bookworm-slim

COPY --from=builder /app/jnq .

COPY ./swagger-ui /swagger-ui

# COPY --from=builder /app/cert.pem .
# COPY --from=builder /app/key.pem .
# COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Expose port 8080 and 8443
EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["./jnq"]