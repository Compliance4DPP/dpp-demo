FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/publish .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/publish /publish
COPY templates /data/templates
ENV HTTP_ADDR=:8080 TEMPLATE_DIR=/data/templates DPP_DIR=/data/dpps
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/publish"]
