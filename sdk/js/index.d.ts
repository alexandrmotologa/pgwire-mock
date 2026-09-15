export interface MockError {
  code: string;
  message: string;
  severity?: string;
  detail?: string;
  hint?: string;
}

export interface Rule {
  id?: string;
  query?: string;
  pattern?: string;
  params?: string[];
  columns?: string[];
  types?: number[];
  rows?: string[][];
  tag?: string;
  error?: MockError;
  latency_ms?: number;
  jitter_ms?: number;
  drop_connection?: boolean;
  fail_after_n?: number;
}

export interface AssertOptions {
  query?: string;
  count?: number;
  minCount?: number;
  exactParams?: string[];
}

export interface AssertResult {
  passed: boolean;
  actual_count: number;
  error?: string;
}

export interface QueryLog {
  timestamp: string;
  query: string;
  params?: string[];
  matched_rule_id?: string;
}

export class PGWireMock {
  constructor(options?: { adminUrl?: string });
  addRule(rule: Rule): Promise<Rule>;
  deleteRule(ruleId: string): Promise<boolean>;
  getRules(): Promise<Rule[]>;
  getQueries(): Promise<QueryLog[]>;
  assertQuery(options: AssertOptions): Promise<AssertResult>;
  reset(): Promise<boolean>;
  health(): Promise<{ status: string; version: string; service: string }>;
}
