FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /server .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /server .
# ponytail: schema.sql not applied here; run migrations separately
VOLUME /app/uploads
EXPOSE 8080
CMD ["./server"]
