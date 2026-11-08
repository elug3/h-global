# hg-server image. The broadcast relay runs ffmpeg, so the runtime is Debian
# slim with ffmpeg rather than a distroless base.
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hg-server ./cmd/hg-server

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 10001 --home /data hg \
 && mkdir -p /data && chown hg /data
COPY --from=build /out/hg-server /usr/local/bin/hg-server
ENV HG_HTTP_ADDR=:8080 HG_STORAGE_DIR=/data
VOLUME /data
EXPOSE 8080 9000/udp
USER hg
ENTRYPOINT ["hg-server"]
