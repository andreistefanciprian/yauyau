FROM golang:1.26-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/frontend ./cmd/server

FROM alpine:3.20
WORKDIR /app

COPY --from=build /out/frontend ./frontend
COPY --from=build /app/templates ./templates
COPY --from=build /app/static ./static

EXPOSE 8080
ENTRYPOINT ["./frontend"]
