FROM golang:1.27-alpine AS build
ARG VERSION=dev-build
RUN apk add --no-cache make curl
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build VERSION=${VERSION}

FROM alpine:3.22
RUN adduser -D -u 10001 inoichi && mkdir -p /app/data && chown inoichi:inoichi /app/data
COPY --from=build /src/inoichi /usr/local/bin/inoichi
USER inoichi
WORKDIR /app
EXPOSE 8080
VOLUME /app/data
ENTRYPOINT ["inoichi", "serve", "--host", "0.0.0.0", "--port", "8080", "--data-dir", "/app/data"]
