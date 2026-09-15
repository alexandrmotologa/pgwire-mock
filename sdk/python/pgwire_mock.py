"""
PGWireMock - Zero-dependency Python client and PyTest fixtures for PGWire-Mock.
Compatible with Python 3.8+ using only standard library modules.
"""

import json
import urllib.request
import urllib.error
from typing import Any, Dict, List, Optional


class PGWireMock:
    """Client for PGWire-Mock Admin and Assertion REST API."""

    def __init__(self, admin_url: str = "http://localhost:8080"):
        self.admin_url = admin_url.rstrip("/")

    def _request(self, method: str, path: str, payload: Optional[Dict[str, Any]] = None) -> Any:
        url = f"{self.admin_url}{path}"
        data = None
        headers = {}

        if payload is not None:
            data = json.dumps(payload).encode("utf-8")
            headers["Content-Type"] = "application/json"

        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        with urllib.request.urlopen(req, timeout=5) as resp:
            body = resp.read().decode("utf-8")
            if body:
                return json.loads(body)
            return None

    def add_rule(self, rule: Dict[str, Any]) -> Dict[str, Any]:
        """Dynamically registers a mock rule."""
        return self._request("POST", "/api/rules", payload=rule)

    def delete_rule(self, rule_id: str) -> bool:
        """Deletes a mock rule by ID."""
        try:
            self._request("DELETE", f"/api/rules/{rule_id}")
            return True
        except urllib.error.HTTPError:
            return False

    def get_rules(self) -> List[Dict[str, Any]]:
        """Returns all registered mock rules."""
        data = self._request("GET", "/api/rules")
        return data.get("rules", [])

    def get_queries(self) -> List[Dict[str, Any]]:
        """Returns recorded query history."""
        data = self._request("GET", "/api/queries")
        return data.get("queries", [])

    def assert_query(
        self,
        query: str = "",
        count: Optional[int] = None,
        min_count: Optional[int] = None,
        exact_params: Optional[List[str]] = None,
    ) -> Dict[str, Any]:
        """Asserts that a query was executed according to expectations."""
        body: Dict[str, Any] = {"query": query}
        if count is not None:
            body["count"] = count
        if min_count is not None:
            body["min_count"] = min_count
        if exact_params is not None:
            body["exact_params"] = exact_params

        return self._request("POST", "/api/assert", payload=body)

    def reset(self) -> bool:
        """Clears query execution history."""
        self._request("POST", "/api/reset")
        return True

    def health(self) -> Dict[str, Any]:
        """Returns mock server health information."""
        return self._request("GET", "/health")


try:
    import pytest

    @pytest.fixture
    def pgwire():
        """PyTest fixture providing an auto-resetting PGWireMock client."""
        client = PGWireMock()
        client.reset()
        yield client
        client.reset()

except ImportError:
    pass
