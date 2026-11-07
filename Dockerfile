# Builds the llama-swap.ai site into a small image for Fly.io.
#
# `make build` looks up llama-swap's latest commit on main and builds against
# it, so every image carries the current llama-swap docs and source.

FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The commit being built; .git is not in the build context.
ARG VERSION=dev
RUN make build VERSION=$VERSION

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /src/build/site-llama-swap /site-llama-swap
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/site-llama-swap", "-listen", ":8080"]
