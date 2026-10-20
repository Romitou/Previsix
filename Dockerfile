FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /previsix .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /previsix /previsix
ENV PREVISIX_SERVER_HOST=0.0.0.0
EXPOSE 3000
ENTRYPOINT ["/previsix"]
