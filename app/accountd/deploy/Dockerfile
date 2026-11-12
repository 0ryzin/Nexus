# Build from the repository's app/ folder:  docker build -f accountd/deploy/Dockerfile -t nexus-accountd .
FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /nexus-accountd ./cmd/nexus-accountd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /nexus-accountd /nexus-accountd
VOLUME /data
EXPOSE 8480
ENTRYPOINT ["/nexus-accountd", "-data", "/data", "-listen", ":8480", "-trust-proxy"]
