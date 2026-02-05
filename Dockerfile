FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o municourt .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/municourt /usr/local/bin/municourt
COPY data/ /data/
EXPOSE 8080
CMD ["municourt", "web", "-dir", "/data", "-port", "8080"]
