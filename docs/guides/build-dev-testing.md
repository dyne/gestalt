# Build, dev, and testing

## Prerequisites

Use tool versions from `mise.toml` (Go + Node).

## Build

```sh
npm i
make
make install
```

## Testing

```sh
make test
```

Optional direct commands:

```sh
go test ./...
cd frontend && npm test -- --run
```

See `TESTING.md` for platform notes and additional checks.
