# HTTP Admin and Assertion API Reference

PGWire-Mock includes an embedded HTTP server (default port `8080`) that allows test runners (such as Jest, PyTest, or Go test) to inspect database activity, inject dynamic mock rules, and assert query execution during CI pipeline runs.

## Endpoints

### 1. Health check

```http
GET /health
```

Returns service status.

**Response (200 OK):**
```json
{
  "service": "pgwire-mock",
  "status": "healthy",
  "version": "1.0.0"
}
```

---

### 2. Prometheus metrics

```http
GET /metrics
```

Returns standard Prometheus text metrics.

**Response (200 OK):**
```text
# HELP pgwire_active_connections Current active PostgreSQL client connections
# TYPE pgwire_active_connections gauge
pgwire_active_connections 2
# HELP pgwire_total_queries Cumulative total queries processed
# TYPE pgwire_total_queries counter
pgwire_total_queries 45
# HELP pgwire_mock_rules_total Active mock rules registered
# TYPE pgwire_mock_rules_total gauge
pgwire_mock_rules_total 8
```

---

### 3. List active mock rules

```http
GET /api/rules
```

Returns all currently loaded mock rules.

**Response (200 OK):**
```json
{
  "count": 1,
  "rules": [
    {
      "id": "rule-1",
      "query": "SELECT id, email FROM users WHERE id = $1",
      "params": ["1"],
      "columns": ["id", "email"],
      "types": [23, 25],
      "rows": [["1", "alex@mtlg.site"]],
      "tag": "SELECT 1"
    }
  ]
}
```

---

### 4. Dynamically inject mock rule

```http
POST /api/rules
Content-Type: application/json
```

Adds a high-priority mock rule to the server during a test run without restarting.

**Request body:**
```json
{
  "id": "custom-rule-1",
  "query": "SELECT count(*) FROM orders WHERE status = $1",
  "params": ["shipped"],
  "columns": ["count"],
  "types": [20],
  "rows": [["15"]],
  "tag": "SELECT 1"
}
```

**Response (201 Created):** Returns the created rule JSON.

---

### 5. Delete mock rule

```http
DELETE /api/rules/{id}
```

Removes a specific rule by its identifier.

**Response (200 OK):**
```json
{
  "deleted": "custom-rule-1"
}
```

---

### 6. Retrieve query history

```http
GET /api/queries
```

Returns the chronological list of queries received by the mock server.

**Response (200 OK):**
```json
{
  "count": 2,
  "queries": [
    {
      "timestamp": "2026-09-15T20:45:00Z",
      "query": "SELECT 1",
      "params": [],
      "matched_rule_id": "builtin-select-1"
    },
    {
      "timestamp": "2026-09-15T20:45:02Z",
      "query": "SELECT id, email FROM users WHERE id = $1",
      "params": ["1"],
      "matched_rule_id": "user-lookup-1"
    }
  ]
}
```

---

### 7. Run query assertion

```http
POST /api/assert
Content-Type: application/json
```

Validates query call counts and parameter values.

**Request body:**
```json
{
  "query": "SELECT id, email FROM users WHERE id = $1",
  "count": 1,
  "exact_params": ["1"]
}
```

**Response (200 OK - Passed):**
```json
{
  "passed": true,
  "actual_count": 1
}
```

**Response (200 OK - Failed):**
```json
{
  "passed": false,
  "actual_count": 0,
  "error": "assertion failed: expected 1 calls for \"SELECT id, email FROM users WHERE id = $1\", but got 0"
}
```

---

### 8. Reset query history

```http
POST /api/reset
```

Clears the recorded query logs between tests.

**Response (200 OK):**
```json
{
  "status": "cleared"
}
```
