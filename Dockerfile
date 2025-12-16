FROM golang:1.22-alpine AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o calendar-gateway

FROM gcr.io/distroless/base-debian12

WORKDIR /app
COPY --from=build /app/calendar-gateway .
EXPOSE 3000

USER nonroot:nonroot
CMD ["/app/calendar-gateway"]
