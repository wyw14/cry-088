# Middleware boundary

HTTP middleware is kept under `internal/transport/http` so request context and
security behavior remain close to the transport owner. This directory reserves
the application boundary for future protocol adapters.
