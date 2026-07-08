FROM golang:1.26-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/backend-api ./cmd/server

FROM alpine:3.20
WORKDIR /app

COPY --from=build /out/backend-api ./backend-api
COPY --from=build /app/migrations ./migrations

EXPOSE 8081
ENTRYPOINT ["./backend-api"]
