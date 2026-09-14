# syntax=docker/dockerfile:1
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/dummy-jfrog .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/dummy-jfrog /dummy-jfrog
COPY catalog.yaml /catalog/catalog.yaml
ENV DUMMY_JFROG_ADDR=:8081 \
    DUMMY_JFROG_CATALOG=/catalog/catalog.yaml
EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/dummy-jfrog"]
