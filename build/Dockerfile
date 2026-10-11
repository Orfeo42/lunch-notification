FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine3.24 AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
RUN apk add --no-cache tzdata
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} go build -trimpath -ldflags="-s -w" -o /out/lunch-notification ./cmd/lunch-notification
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build /out/lunch-notification /lunch-notification
USER nonroot:nonroot
ENTRYPOINT ["/lunch-notification"]
CMD ["serve"]
