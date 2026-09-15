# PGWire-Mock Architecture

PGWire-Mock is a lightweight socket server and proxy that implements the PostgreSQL Frontend/Backend Protocol 3.0 over TCP. It allows client drivers and ORMs to communicate with a mock server without running a real PostgreSQL database instance.

## System components

The system consists of five decoupled layers:

```
[ PostgreSQL Client (psql, pgx, psycopg, node-pg) ]
                      |
                 TCP Port 5432
                      v
            [ Server Listener ]
                      |
           [ Connection Handler ]
           /          |          \
          v           v           v
  [ Protocol ]    [ Mock ]     [ Admin API ]
  (Reader/Writer) (Matcher)   (Port 8080)
```

1. **Protocol Engine (`internal/protocol`)**: Handles binary serialization and deserialization of frontend and backend packets according to PostgreSQL 3.0 specification.
2. **Mock Rule Engine (`internal/mock`)**: Evaluates incoming queries against loaded rules using exact match, normalized SQL, regex, and parameter bindings.
3. **Connection Handler (`internal/server`)**: Manages client connection state, including the SSL handshake, startup authentication, simple query processing, and extended query transactions.
4. **Admin and Assertion API (`internal/admin`)**: Runs an embedded HTTP REST server allowing automated test suites to register dynamic mock rules, inspect query logs, and run assertions.
5. **Proxy and Recorder (`internal/proxy`)**: Forwards traffic to an upstream PostgreSQL instance and serializes packets into replayable `.pgtape` files.

## Protocol lifecycle

### 1. SSL negotiation and startup handshake

When a client connects over TCP, it typically starts by sending an 8-byte `SSLRequest` packet (`code 80877103`). PGWire-Mock responds with the single byte `'N'` to indicate that SSL is not supported, instructing the driver to fall back to plaintext.

The client then transmits its `StartupMessage` containing protocol version 3.0 and connection parameters (such as `user`, `database`, and `client_encoding`).

Upon receiving the startup message, PGWire-Mock transmits:

- `AuthenticationOk` (`'R'`): Informs the client that authentication succeeded without requiring a password.
- `ParameterStatus` (`'S'`): Emits baseline session variables (`server_version`, `client_encoding`, `TimeZone`, `integer_datetimes`).
- `BackendKeyData` (`'K'`): Provides a process ID and cancellation secret key.
- `ReadyForQuery` (`'Z'`): Indicates that the backend is idle (`'I'`) and ready to accept queries.

### 2. Simple query processing ('Q')

In the simple query protocol, the client sends a message starting with `'Q'` containing the SQL text.

1. The server extracts and trims the SQL query.
2. The query is passed to the mock engine.
3. If the matched rule specifies artificial latency or jitter, the handler sleeps for the requested duration.
4. If the rule specifies an error, an `ErrorResponse` (`'E'`) packet is sent with the configured SQLSTATE code and message.
5. If the rule returns columns and rows:
   - A `RowDescription` (`'T'`) packet describes column names and type OIDs.
   - One or more `DataRow` (`'D'`) packets convey row values.
6. A `CommandComplete` (`'C'`) packet confirms execution completion.
7. A `ReadyForQuery` (`'Z'`) packet signals readiness for the next command.

### 3. Extended query processing

Modern database drivers and ORMs use the extended query protocol for prepared statements and parameter bindings:

- **Parse (`'P'`)**: Prepares a SQL template containing placeholders (`$1`, `$2`) and caches it under a statement name.
- **Bind (`'B'`)**: Binds parameter byte slices to a prepared statement and creates a portal.
- **Describe (`'D'`)**: Returns column and parameter metadata (`RowDescription` or `ParameterDescription`).
- **Execute (`'E'`)**: Evaluates the portal query with bound parameters against the mock rules and streams back data rows.
- **Sync (`'S'`)**: Completes the pipeline and issues a `ReadyForQuery` packet.

### 4. Admin and test assertions

Automated integration tests can interact with the embedded HTTP server:

- `POST /api/rules`: Inject mock rules dynamically before a specific test case runs.
- `GET /api/queries`: Inspect all queries executed by the application during the test.
- `POST /api/assert`: Verify that a query ran a specific number of times or received expected parameter values.
- `POST /api/reset`: Clear query history between test runs to ensure test isolation.
