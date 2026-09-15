/**
 * PGWireMock - Lightweight JavaScript / TypeScript client for PGWire-Mock Admin & Assertion API.
 * Supports Jest, Vitest, Mocha, and Node.js test runners with zero external dependencies.
 */
class PGWireMock {
  /**
   * @param {Object} [options]
   * @param {string} [options.adminUrl='http://localhost:8080'] - HTTP Admin API base URL
   */
  constructor({ adminUrl = 'http://localhost:8080' } = {}) {
    this.adminUrl = adminUrl.replace(/\/+$/, '');
  }

  /**
   * Registers a new mock rule dynamically.
   * @param {Object} rule - Mock rule definition
   * @param {string} [rule.query] - Exact or normalized SQL query
   * @param {string} [rule.pattern] - Regex matching pattern
   * @param {string[]} [rule.params] - Expected parameter bindings ($1, $2)
   * @param {string[]} [rule.columns] - Result column names
   * @param {string[][]} [rule.rows] - Row values
   * @param {number} [rule.latency_ms] - Artificial latency injection in ms
   * @param {number} [rule.jitter_ms] - Additional random jitter in ms
   * @param {boolean} [rule.drop_connection] - Drop TCP connection on match
   * @param {Object} [rule.error] - Mock error { code: "42P01", message: "..." }
   * @returns {Promise<Object>} Created rule
   */
  async addRule(rule) {
    const res = await fetch(`${this.adminUrl}/api/rules`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(rule),
    });
    if (!res.ok) {
      const errText = await res.text();
      throw new Error(`Failed to add rule (${res.status}): ${errText}`);
    }
    return res.json();
  }

  /**
   * Deletes a mock rule by its ID.
   * @param {string} ruleId
   * @returns {Promise<boolean>}
   */
  async deleteRule(ruleId) {
    const res = await fetch(`${this.adminUrl}/api/rules/${encodeURIComponent(ruleId)}`, {
      method: 'DELETE',
    });
    return res.ok;
  }

  /**
   * Fetches all registered mock rules.
   * @returns {Promise<Object[]>}
   */
  async getRules() {
    const res = await fetch(`${this.adminUrl}/api/rules`);
    const data = await res.json();
    return data.rules || [];
  }

  /**
   * Fetches recorded queries.
   * @returns {Promise<Object[]>}
   */
  async getQueries() {
    const res = await fetch(`${this.adminUrl}/api/queries`);
    const data = await res.json();
    return data.queries || [];
  }

  /**
   * Asserts whether a specific query was executed with given criteria.
   * @param {Object} criteria
   * @param {string} [criteria.query] - Query or substring to assert
   * @param {number} [criteria.count] - Expected exact count
   * @param {number} [criteria.minCount] - Minimum expected count
   * @param {string[]} [criteria.exactParams] - Expected parameter bindings
   * @returns {Promise<{ passed: boolean, actual_count: number, error?: string }>}
   */
  async assertQuery({ query = '', count, minCount, exactParams } = {}) {
    const body = { query };
    if (typeof count === 'number') body.count = count;
    if (typeof minCount === 'number') body.min_count = minCount;
    if (Array.isArray(exactParams)) body.exact_params = exactParams;

    const res = await fetch(`${this.adminUrl}/api/assert`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });

    return res.json();
  }

  /**
   * Clears all recorded queries from memory.
   * @returns {Promise<boolean>}
   */
  async reset() {
    const res = await fetch(`${this.adminUrl}/api/reset`, {
      method: 'POST',
    });
    return res.ok;
  }

  /**
   * Checks the health of the mock server.
   * @returns {Promise<{ status: string, version: string }>}
   */
  async health() {
    const res = await fetch(`${this.adminUrl}/health`);
    return res.json();
  }
}

module.exports = { PGWireMock };
