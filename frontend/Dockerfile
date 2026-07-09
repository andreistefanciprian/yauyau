FROM golang:1.26-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/frontend ./cmd/server

FROM scratch
WORKDIR /app

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/frontend ./frontend
COPY --from=build /app/templates ./templates
COPY --from=build /app/static ./static

USER 65532:65532

EXPOSE 8080
ENTRYPOINT ["./frontend"]
