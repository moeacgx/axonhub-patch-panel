FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go test ./... && CGO_ENABLED=0 GOOS=linux go build -o /out/axonhub-patch ./cmd/axonhub-patch

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/axonhub-patch /app/axonhub-patch
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/axonhub-patch"]
