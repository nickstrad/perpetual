# perpetual

perpetual currently registers and inspects durable machine intent. A registration
has state `registered` and `provisioned: false`. It does not start a virtual machine.

The service stores immutable registrations in PostgreSQL. Retrying the same request
ID and parameters returns the original machine identity, including after a service
restart. Changing the parameters under that ID, or claiming an already registered
name, returns a conflict.

## Build and run

Use Go 1.26.8 and a dedicated PostgreSQL 18 database. Build the two binaries:

```sh
make build
```

For local development, Docker Compose supplies the pinned PostgreSQL image and a
persistent development volume:

```sh
make dev-db-up
eval "$(make -s dev-db-env)"
bin/agent-plane
```

In another terminal, use the CLI commands below. `make dev-db-status` and
`make dev-db-logs` inspect this database. `make dev-db-down` stops it and keeps the
data; `make dev-db-reset` deletes its development data. Override
`PERPETUAL_DEV_PORT=54330` if port 54329 is occupied. Integration tests use separate
containers and disposable databases. Run `make help` to list build and test targets.

Configure the service with the variables in [config/example.env](config/example.env).
Set `PERPETUAL_DATABASE_URL` to your dedicated database. The service applies its
versioned migration on startup and verifies the persisted registration limit.
It takes an exclusive lock in `PERPETUAL_RUNTIME_DIR`, which defaults to
`/run/perpetual`, and listens on `127.0.0.1:7777` by default.
The CLI uses `PERPETUAL_API_URL`, defaulting to `http://127.0.0.1:7777`.

```sh
bin/agent-plane
bin/perpetual machine register \
  --request-id 11111111-1111-4111-8111-111111111111 \
  --name demo-a --image base
bin/perpetual registration inspect 11111111-1111-4111-8111-111111111111
```

The registration response contains the machine ID for `perpetual machine inspect`.
Registration defaults are one vCPU, 512 MiB memory and 1,024 MiB disk. Image names
are recorded as intent; this slice does not check or provision image files.

Successful commands print one JSON object to stdout. If `--request-id` is omitted,
the CLI generates an ID and prints it to stderr before sending the request. Retain
that ID. If the reply is lost, inspect or retry with the same ID and parameters.
An absent read does not prove an in-flight registration aborted.

| Exit | Meaning |
| --- | --- |
| 0 | Success |
| 2 | Invalid command or input |
| 3 | Request or name conflict |
| 4 | Registration not found |
| 5 | Capacity, busy, or unavailable |
| 6 | Mutation outcome unknown; retain the original request ID |

## Verification and design

[TESTING.md](TESTING.md) describes executable checks, isolated PostgreSQL fixtures,
simulation assumptions, and coverage limitations. Integration checks own their
disposable database server and never borrow an operator's database.

The [slice 1 plan](docs/plans/durable_requests/plan.md) specifies the
registration and recovery contracts. The [MVP breakdown](docs/plans/README.md)
sets the implementation order; [project direction](docs/knowledge/project-direction.md)
records delivery status.
