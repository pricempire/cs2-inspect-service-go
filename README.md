# CS2 Inspect Service

This is a simple service that inspects CS2 items.

## Running the service

```bash
GOLANG_PROTOBUF_REGISTRATION_CONFLICT=warn go run .
```

## Using the service

```bash
curl -X GET "http://localhost:3000/inspect?link=steam://rungame/730/76561202255233023/+csgo_econ_action_preview%20S76561198023809011A39999055096D14136313962912534216"
```

## Health check

```bash
curl -X GET "http://localhost:3000/health"
```
