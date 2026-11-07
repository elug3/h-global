# hg-server image: a static Go binary on a distroless base.
FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hg-server ./cmd/hg-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/hg-server /hg-server
ENV HG_HTTP_ADDR=:8080 HG_STORAGE_DIR=/data
VOLUME /data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/hg-server"]
