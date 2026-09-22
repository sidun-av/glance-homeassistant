FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/glance-homeassistant . && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/glance-homeassistant /glance-homeassistant
COPY config.docker-default.yml /config.yml
# /data is where the floorplan editor saves; owned by the nonroot user the
# image runs as, so a named volume mounted there inherits that ownership.
COPY --from=build --chown=nonroot:nonroot /out/data /data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/glance-homeassistant"]
