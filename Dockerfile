FROM golang:1.23-alpine AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /tinykmetrics ./cmd/tinykmetrics/main.go

FROM scratch
COPY --from=build /tinykmetrics /
COPY --from=build /app/static /static
USER 65534:65534
CMD ["/tinykmetrics"]