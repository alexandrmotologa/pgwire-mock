# pgwire-mock (Python SDK)

Zero-dependency Python client and PyTest fixtures for PGWire-Mock.

Allows test suites to dynamically register mock rules, assert database queries, and simulate database chaos without external databases or Docker containers.

## Installation

```bash
pip install pgwire-mock
```

## Quick Start with PyTest

```python
import psycopg2
import pytest
from pgwire_mock import PGWireMock, pgwire


def test_user_retrieval(pgwire: PGWireMock):
    # Register dynamic mock rule
    pgwire.add_rule({
        "id": "get-user-by-id",
        "query": "SELECT id, name FROM users WHERE id = $1",
        "params": ["42"],
        "columns": ["id", "name"],
        "rows": [["42", "Grace Hopper"]],
        "tag": "SELECT 1",
    })

    # Run application code using psycopg2
    conn = psycopg2.connect("postgresql://postgres:postgres@localhost:5432/testdb")
    cur = conn.cursor()
    cur.execute("SELECT id, name FROM users WHERE id = %s", ("42",))
    row = cur.fetchone()

    assert row == ("42", "Grace Hopper")

    # Assert query execution
    result = pgwire.assert_query(
        query="SELECT id, name FROM users",
        count=1,
        exact_params=["42"]
    )
    assert result["passed"] is True

    cur.close()
    conn.close()
```

## API Methods

- `add_rule(rule: dict)`: Registers a mock rule with custom query, columns, rows, latency, or error.
- `delete_rule(rule_id: str)`: Deletes a mock rule by ID.
- `get_rules()`: Lists registered mock rules.
- `get_queries()`: Retrieves recorded queries and parameters.
- `assert_query(query=..., count=..., min_count=..., exact_params=...)`: Validates execution history.
- `reset()`: Resets query logs.
- `health()`: Verifies mock server health.
