FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
RUN apk add --no-cache make curl
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH VERSION=dev-build
RUN make assets && make build-for GOOS=$TARGETOS GOARCH=$TARGETARCH VERSION=${VERSION} && \
    mv inoichi-$TARGETOS-$TARGETARCH /src/inoichi

FROM alpine:3.22
RUN addgroup -g 10001 -S inoichi && adduser -u 10001 -S -G inoichi inoichi && \
    mkdir -p /app/data && chown 10001:10001 /app/data
COPY --from=build --chown=10001:10001 /src/inoichi /usr/local/bin/inoichi
USER 10001:10001
WORKDIR /app
EXPOSE 8080
VOLUME /app/data
ENTRYPOINT ["inoichi"]
CMD ["serve", "--host", "0.0.0.0", "--port", "8080", "--data-dir", "/app/data"]
