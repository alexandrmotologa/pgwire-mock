# @alexandrmotologa/pgwire-mock

Zero-dependency Node.js and TypeScript client for the PGWire-Mock Admin and Assertion API.

Designed for Jest, Vitest, Mocha, and Node test runners to dynamically register mock rules, assert executed database queries, and clear history between test cases.

## Installation

```bash
npm install --save-dev @alexandrmotologa/pgwire-mock
```

## Quick Start (Vitest / Jest)

```javascript
import { describe, it, beforeEach, expect } from 'vitest';
import { PGWireMock } from '@alexandrmotologa/pgwire-mock';
import { Client } from 'pg';

describe('Order Service Tests', () => {
  const mock = new PGWireMock({ adminUrl: 'http://localhost:8080' });
  let db;

  beforeEach(async () => {
    await mock.reset();
  });

  it('handles user creation and asserts query execution', async () => {
    // 1. Register a dynamic mock rule for this specific test
    await mock.addRule({
      id: 'create-user',
      query: 'INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id',
      columns: ['id'],
      rows: [['{{uuid}}']],
      tag: 'INSERT 0 1',
    });

    // 2. Run application code that connects to localhost:5432
    db = new Client({ connectionString: 'postgresql://postgres:postgres@localhost:5432/testdb' });
    await db.connect();
    const res = await db.query('INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id', [
      'Ada Lovelace',
      'ada@example.com',
    ]);

    expect(res.rows[0].id).toBeDefined();

    // 3. Assert query was executed with exact parameter bindings
    const assertion = await mock.assertQuery({
      query: 'INSERT INTO users',
      count: 1,
      exactParams: ['Ada Lovelace', 'ada@example.com'],
    });

    expect(assertion.passed).toBe(true);
    await db.end();
  });
});
```

## API Reference

- `addRule(rule)`: Registers a new mock rule with query or regex pattern, columns, rows, latency, or injected errors.
- `deleteRule(id)`: Removes a rule by its ID.
- `getRules()`: Returns an array of active mock rules.
- `getQueries()`: Returns all captured queries and parameter bindings.
- `assertQuery({ query, count, minCount, exactParams })`: Verifies query execution counts and bound values.
- `reset()`: Clears query execution history.
- `health()`: Checks mock server health and status.
