FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/storere-api ./cmd/api
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/storere-api /storere-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/storere-api"]
