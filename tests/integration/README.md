# PostgreSQL integration tests

Integration scenarios run against the PostgreSQL service from Docker Compose.
The repositories use transaction-local `FOR UPDATE`, unique constraints and
optimistic version predicates; CI can mount this directory for the full suite.
