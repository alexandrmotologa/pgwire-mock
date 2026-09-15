# PostgreSQL Frontend/Backend Protocol 3.0 Reference

This document outlines the binary wire format implemented by PGWire-Mock according to the PostgreSQL 3.0 protocol specification.

## Packet header format

All messages following the initial handshake use a 5-byte header:

- **Byte 0**: Message type byte (`'Q'`, `'P'`, `'B'`, `'D'`, `'E'`, `'S'`, `'T'`, `'C'`, etc.)
- **Bytes 1 to 4**: 32-bit big-endian integer representing the length of the packet, including the 4 bytes of the length field itself, but excluding the 1-byte message type.

## Supported message types

### Handshake messages

| Type | Identifier | Direction | Description |
|------|------------|-----------|-------------|
| SSLRequest | `80877103` | Client -> Server | Asks if SSL encryption is supported. Server replies `'N'` to fall back to unencrypted TCP. |
| StartupMessage | `196608` | Client -> Server | Protocol version 3.0 followed by null-terminated key-value pairs (`user`, `database`, etc.). |
| AuthenticationOk | `'R'` | Server -> Client | Sends 4-byte auth type `0` indicating successful authentication without password. |
| ParameterStatus | `'S'` | Server -> Client | Emits runtime parameters (`server_version`, `client_encoding`, `TimeZone`, `DateStyle`). |
| BackendKeyData | `'K'` | Server -> Client | 32-bit backend process ID and 32-bit cancellation key. |
| ReadyForQuery | `'Z'` | Server -> Client | Single byte transaction status indicator: `'I'` (idle), `'T'` (in transaction), or `'E'` (in failed transaction). |

### Simple query messages

| Type | Identifier | Direction | Description |
|------|------------|-----------|-------------|
| Query | `'Q'` | Client -> Server | Contains a null-terminated SQL query string. |
| RowDescription | `'T'` | Server -> Client | Defines column count, column names, type OIDs, sizes, and format codes. |
| DataRow | `'D'` | Server -> Client | Contains column count and length-prefixed column value byte slices (`-1` for SQL NULL). |
| CommandComplete | `'C'` | Server -> Client | Confirms command completion with a tag string (e.g. `SELECT 1`, `INSERT 0 1`, `SET`). |
| ErrorResponse | `'E'` | Server -> Client | Reports errors with severity, SQLSTATE code, message, detail, and hint fields. |
| EmptyQueryResponse | `'I'` | Server -> Client | Returned when an empty string or semicolon query is submitted. |

### Extended query messages

| Type | Identifier | Direction | Description |
|------|------------|-----------|-------------|
| Parse | `'P'` | Client -> Server | Prepares statement name, query text with parameter placeholders (`$1`, `$2`), and parameter types. |
| ParseComplete | `'1'` | Server -> Client | Confirms that statement preparation succeeded. |
| Bind | `'B'` | Client -> Server | Associates parameter values with a prepared statement to form a portal. |
| BindComplete | `'2'` | Server -> Client | Confirms that parameter binding succeeded. |
| Describe | `'D'` | Client -> Server | Asks for parameter types or result column description for a statement or portal. |
| Execute | `'E'` | Client -> Server | Executes the portal and returns row data. |
| Sync | `'S'` | Client -> Server | Concludes an extended query cycle and prompts the server to return `ReadyForQuery`. |
| Close | `'C'` | Client -> Server | Closes a portal or prepared statement. |
| CloseComplete | `'3'` | Server -> Client | Confirms closure of a portal or statement. |

## Standard PostgreSQL type OIDs

| OID | Type name | Internal size | Description |
|-----|-----------|---------------|-------------|
| 16 | `bool` | 1 byte | Boolean value (`t` or `f`) |
| 17 | `bytea` | variable | Binary byte array |
| 20 | `int8` | 8 bytes | 64-bit signed integer (bigint) |
| 21 | `int2` | 2 bytes | 16-bit signed integer (smallint) |
| 23 | `int4` | 4 bytes | 32-bit signed integer (integer) |
| 25 | `text` | variable | Variable-length string |
| 114 | `json` | variable | JSON document |
| 700 | `float4` | 4 bytes | Single-precision floating point |
| 701 | `float8` | 8 bytes | Double-precision floating point |
| 1043 | `varchar` | variable | Variable-length string with optional limit |
| 1082 | `date` | 4 bytes | Calendar date (`YYYY-MM-DD`) |
| 1114 | `timestamp` | 8 bytes | Date and time without timezone |
| 1184 | `timestamptz` | 8 bytes | Date and time with timezone |
| 2950 | `uuid` | 16 bytes | Universally unique identifier |
| 3802 | `jsonb` | variable | Binary JSON document |
