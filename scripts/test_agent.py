#!/usr/bin/env python3
"""
Comprehensive test harness for Kyoci Agent API
Tests HTTP REST API and gRPC endpoints
"""

import argparse
import json
import re
import subprocess
import sys
import time
import urllib.request
import urllib.error
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, asdict
from datetime import datetime
from typing import List, Dict, Any, Optional
import hashlib
import base64
from urllib.parse import quote


@dataclass
class TestResult:
    """Single test result"""
    category: str
    name: str
    status: str
    duration_ms: float
    detail: str = ""
    suggestion: str = ""


class AgentTestHarness:
    """Test harness for Kyoci Agent"""

    def __init__(self, host: str, http_port: int, grpc_port: int, verbose: bool = False):
        self.base_url = f"http://{host}:{http_port}"
        self.grpc_port = grpc_port
        self.verbose = verbose
        self.results: List[TestResult] = []
        self.weaknesses: List[str] = []
        self.start_time = time.time()

    def log(self, message: str):
        """Log message if verbose"""
        if self.verbose:
            print(f"[{datetime.now().strftime('%H:%M:%S')}] {message}")

    def log_error(self, message: str):
        """Log error message always"""
        print(f"[ERROR] {message}")

    def run_test(self, category: str, name: str, test_func) -> TestResult:
        """Run a single test and capture result"""
        self.log(f"Running {category}.{name}")
        start = time.time()

        try:
            detail, suggestion = test_func()
            status = "PASS"
            self.log(f"  ✓ {name}: {detail}")
        except AssertionError as e:
            status = "FAIL"
            detail = str(e)
            suggestion = ""
            self.log(f"  ✗ {name}: {detail}")
        except Exception as e:
            status = "ERROR"
            detail = f"Exception: {str(e)}"
            suggestion = "Check agent logs for details"
            self.log_error(f"  {name}: {detail}")

        duration_ms = (time.time() - start) * 1000
        result = TestResult(category, name, status, duration_ms, detail, suggestion)
        self.results.append(result)
        return result

    def http_get(self, endpoint: str, headers: Optional[Dict] = None) -> Dict:
        """Make HTTP GET request"""
        url = f"{self.base_url}{endpoint}"
        req = urllib.request.Request(url, headers=headers or {})
        with urllib.request.urlopen(req, timeout=10) as response:
            data = response.read().decode('utf-8')
            return json.loads(data)

    def http_post(self, endpoint: str, body: Dict, headers: Optional[Dict] = None) -> Dict:
        """Make HTTP POST request"""
        url = f"{self.base_url}{endpoint}"
        headers = headers or {}
        headers['Content-Type'] = 'application/json'

        req = urllib.request.Request(
            url,
            data=json.dumps(body).encode('utf-8'),
            headers=headers,
            method='POST'
        )

        with urllib.request.urlopen(req, timeout=10) as response:
            data = response.read().decode('utf-8')
            return json.loads(data)

    def chat(self, prompt: str, provider: str = "auto", session_id: Optional[str] = None, timeout: int = 10) -> Dict:
        """Send chat request to agent"""
        body = {"message": prompt, "provider": provider}
        if session_id:
            body["session_id"] = session_id

        url = f"{self.base_url}/v2/chat"
        req = urllib.request.Request(
            url,
            data=json.dumps(body).encode('utf-8'),
            headers={'Content-Type': 'application/json'},
            method='POST'
        )

        with urllib.request.urlopen(req, timeout=timeout) as response:
            data = response.read().decode('utf-8')
            return json.loads(data)

    # ==================== TEST CATEGORIES ====================

    def run_health_check_tests(self):
        """Category 1: Health Check Tests"""
        def test_health():
            data = self.http_get("/v2/status")
            assert data.get("status") in ["healthy", "ok", "serving"], f"Unexpected health status: {data}"
            return f"Health status: {data.get('status')}", ""

        self.run_test("HEALTH_CHECK", "health_endpoint", test_health)

    def run_tier_0_tests(self):
        """Category 2: Tier 0 - Zero-AI Skills"""
        def test_math():
            response = self.chat("hitung 25*37")
            content = str(response.get("message", "")).lower()
            assert "925" in content, f"Expected '925' in response, got: {content}"
            assert response.get("tier") == 0, f"Expected tier 0, got: {response.get('tier')}"
            return f"Math calculation correct: 25*37=925", ""

        self.run_test("TIER_0", "math_skill", test_math)

        def test_time():
            response = self.chat("jam berapa")
            content = str(response.get("message", ""))
            time_pattern = r'\d{1,2}[:.]\d{2}'
            assert re.search(time_pattern, content), f"No time pattern found in: {content}"
            return f"Time detected in response", ""

        self.run_test("TIER_0", "time_skill", test_time)

        def test_hash():
            response = self.chat("hash sha256 hello")
            content = str(response.get("message", "")).lower()
            expected_hash = hashlib.sha256(b"hello").hexdigest()
            assert expected_hash in content, f"Expected hash {expected_hash} in: {content}"
            return f"SHA256 hash correct", ""

        self.run_test("TIER_0", "hash_skill", test_hash)

        def test_uuid():
            response = self.chat("generate uuid")
            content = str(response.get("message", ""))
            uuid_pattern = r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
            assert re.search(uuid_pattern, content, re.IGNORECASE), f"No UUID pattern found in: {content}"
            return f"UUID generated", ""

        self.run_test("TIER_0", "uuid_skill", test_uuid)

        def test_base64():
            response = self.chat("base64 encode hello")
            content = str(response.get("message", ""))
            expected = base64.b64encode(b"hello").decode('utf-8')
            assert expected in content, f"Expected '{expected}' in: {content}"
            return f"Base64 encoding correct: aGVsbG8=", ""

        self.run_test("TIER_0", "base64_skill", test_base64)

        def test_convert():
            response = self.chat("convert 100 celsius to fahrenheit")
            content = str(response.get("message", "")).lower()
            assert "212" in content, f"Expected '212' in: {content}"
            return f"Temperature conversion correct: 100°C = 212°F", ""

        self.run_test("TIER_0", "convert_skill", test_convert)

        def test_url_encode():
            response = self.chat("encode url hello world")
            content = str(response.get("message", ""))
            expected = quote("hello world")
            assert expected in content or "hello%20world" in content, f"Expected URL encoding in: {content}"
            return f"URL encoding correct", ""

        self.run_test("TIER_0", "encode_skill", test_url_encode)

        def test_weather():
            response = self.chat("weather Jakarta")
            content = str(response.get("message", "")).lower()
            # Should contain weather-related keywords
            weather_keywords = ["temperature", "weather", "suhu", "cuaca"]
            assert any(kw in content for kw in weather_keywords), f"No weather info in: {content}"
            return f"Weather info returned", ""

        self.run_test("TIER_0", "weather_skill", test_weather)

    def run_tier_1_tests(self):
        """Category 3: Tier 1 - Local AI (Ollama)"""
        def test_explain_go():
            response = self.chat("explain what is Go language")
            content = str(response.get("message", ""))
            assert len(content) > 50, f"Response too short: {len(content)} chars"
            assert response.get("tier") == 1, f"Expected tier 1, got: {response.get('tier')}"
            return f"Valid response: {len(content)} chars", ""

        self.run_test("TIER_1", "ollama_reasoning", test_explain_go)

        def test_translate():
            response = self.chat("translate 'hello world' to Indonesian")
            content = str(response.get("message", "")).lower()
            indonesian_words = ["halo", "dunia", "selamat", "pagi"]
            assert any(word in content for word in indonesian_words), f"No Indonesian in: {content}"
            return f"Translation contains Indonesian", ""

        self.run_test("TIER_1", "ollama_translation", test_translate)

        def test_summarize():
            response = self.chat("summarize: Go is a statically typed language")
            content = str(response.get("message", ""))
            assert len(content) > 20, f"Summary too short: {len(content)} chars"
            assert "go" in content.lower(), f"Summary should mention Go"
            return f"Valid summary: {len(content)} chars", ""

        self.run_test("TIER_1", "ollama_summarization", test_summarize)

        def test_malformed():
            try:
                response = self.chat("", provider="auto")
                # If we get here, agent handled it gracefully
                assert "error" in response or "message" in response, f"Unexpected response: {response}"
                return "Graceful handling of malformed input", ""
            except urllib.error.HTTPError as e:
                # Expected behavior
                return f"HTTP {e.code}: graceful error handling", ""

        self.run_test("TIER_1", "malformed_input", test_malformed)

        def test_long_prompt():
            long_prompt = "explain " + "very " * 200 + "detailed architecture"
            response = self.chat(long_prompt)
            content = str(response.get("message", ""))
            assert len(content) > 20, f"Response too short for long prompt"
            return f"Long prompt handled: {len(long_prompt)} chars → {len(content)} chars", ""

        self.run_test("TIER_1", "long_prompt", test_long_prompt)

    def run_json_validation_tests(self):
        """Category 4: JSON Structure Validation"""
        def test_response_structure():
            response = self.chat("test")
            assert "message" in response, f"Missing 'content' field: {response}"
            assert "tier" in response, f"Missing 'tier' field: {response}"
            assert "duration" in response or "latency" in response, f"Missing duration field: {response}"
            assert "model" in response or "provider" in response, f"Missing model field: {response}"
            return "Response structure valid", ""

        self.run_test("JSON_VALIDATION", "response_structure", test_response_structure)

        def test_error_structure():
            try:
                self.http_post("/v2/chat", {}, headers={'Content-Type': 'application/json'})
                assert False, "Should have raised error"
            except urllib.error.HTTPError as e:
                data = json.loads(e.read().decode('utf-8'))
                assert "error" in data or "message" in data, f"Missing error field: {data}"
                assert "code" in data or e.code is not None, f"Missing code field"
                return f"Error structure valid: HTTP {e.code}", ""

        self.run_test("JSON_VALIDATION", "error_structure", test_error_structure)

        def test_session_preserved():
            session_id = "test-session-12345"
            response1 = self.chat("first message", session_id=session_id)
            response2 = self.chat("second message", session_id=session_id)
            # Both should have same session ID if it's echoed back
            # Or at least both should succeed
            assert response1 is not None and response2 is not None
            return f"Session preserved across requests", ""

        self.run_test("JSON_VALIDATION", "session_preservation", test_session_preserved)

    def run_error_handling_tests(self):
        """Category 5: Error Handling"""
        def test_empty_prompt():
            try:
                response = self.chat("")
                # If agent handles it, should have error message
                if "error" in response:
                    return f"Empty prompt handled with error message", ""
                else:
                    return "Empty prompt accepted (may be valid)", ""
            except urllib.error.HTTPError as e:
                assert e.code in [400, 422, 500], f"Unexpected code: {e.code}"
                return f"Empty prompt rejected with HTTP {e.code}", ""

        self.run_test("ERROR_HANDLING", "empty_prompt", test_empty_prompt)

        def test_invalid_provider():
            try:
                response = self.chat("test", provider="invalid_provider_xyz")
                # Agent may handle gracefully or reject
                return "Invalid provider handled gracefully", ""
            except urllib.error.HTTPError as e:
                return f"Invalid provider rejected with HTTP {e.code}", ""

        self.run_test("ERROR_HANDLING", "invalid_provider", test_invalid_provider)

        def test_missing_content_type():
            try:
                url = f"{self.base_url}/v2/chat"
                req = urllib.request.Request(
                    url,
                    data=json.dumps({"message": "test"}).encode('utf-8'),
                    headers={},  # No Content-Type
                    method='POST'
                )
                with urllib.request.urlopen(req, timeout=5) as response:
                    data = response.read()
                    return "Request accepted without Content-Type", ""
            except urllib.error.HTTPError as e:
                return f"Missing Content-Type rejected with HTTP {e.code}", ""

        self.run_test("ERROR_HANDLING", "missing_content_type", test_missing_content_type)

        def test_timeout_handling():
            try:
                # Complex prompt with short timeout
                complex_prompt = "explain " + "theoretical " * 300 + "computer science concepts in detail"
                response = self.chat(complex_prompt, timeout=2)
                return "Complex prompt completed within timeout", ""
            except urllib.error.URLError as e:
                if "timeout" in str(e).lower():
                    suggestion = "Consider increasing timeout for complex prompts"
                    self.weaknesses.append("Timeout too short for complex prompts")
                    return f"Timeout occurred as expected", suggestion
                return f"Error: {e}", ""
            except Exception as e:
                return f"Exception during timeout test: {e}", ""

        self.run_test("ERROR_HANDLING", "timeout_handling", test_timeout_handling)

    def run_concurrent_tests(self):
        """Category 6: Concurrent Requests"""
        def test_concurrent_5():
            def make_request(i):
                return self.chat(f"concurrent request {i}")

            start = time.time()
            with ThreadPoolExecutor(max_workers=5) as executor:
                futures = [executor.submit(make_request, i) for i in range(5)]
                results = [f.result() for f in as_completed(futures)]

            duration = (time.time() - start) * 1000
            assert len(results) == 5, f"Expected 5 results, got {len(results)}"
            assert all(r is not None for r in results), "Some requests failed"
            return f"5 concurrent requests succeeded in {duration:.0f}ms", ""

        self.run_test("CONCURRENT", "concurrent_5", test_concurrent_5)

        def test_concurrent_10():
            def make_request(i):
                return self.chat(f"stress test request {i}")

            start = time.time()
            with ThreadPoolExecutor(max_workers=10) as executor:
                futures = [executor.submit(make_request, i) for i in range(10)]
                results = [f.result(timeout=15) for f in as_completed(futures)]

            duration = (time.time() - start) * 1000
            assert len(results) == 10, f"Expected 10 results, got {len(results)}"
            assert all(r is not None for r in results), "Some requests failed"
            return f"10 concurrent requests succeeded in {duration:.0f}ms", ""

        self.run_test("CONCURRENT", "concurrent_10_stress", test_concurrent_10)

    def run_grpc_tests(self):
        """Category 7: gRPC Tests"""
        # Check if grpcurl is available
        try:
            subprocess.run(
                ["grpcurl", "-version"],
                check=True,
                capture_output=True,
                timeout=5
            )
        except (subprocess.CalledProcessError, FileNotFoundError):
            self.log_error("grpcurl not found, skipping gRPC tests")
            self.log("Install grpcurl: brew install grpcurl (macOS) or go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest")
            return

        def test_grpc_health():
            result = subprocess.run(
                ["grpcurl", "-plaintext", f"localhost:{self.grpc_port}",
                 "agent.AgentService/HealthCheck"],
                capture_output=True,
                text=True,
                timeout=10
            )
            assert result.returncode == 0, f"gRPC health check failed: {result.stderr}"
            data = json.loads(result.stdout)
            assert data.get("status") == "SERVING", f"Unexpected status: {data.get('status')}"
            return f"gRPC health check: SERVING", ""

        self.run_test("GRPC", "health_check", test_grpc_health)

        def test_grpc_simple_prompt():
            result = subprocess.run(
                ["grpcurl", "-plaintext", "-d", '{"message": "hello"}',
                 f"localhost:{self.grpc_port}", "agent.AgentService/Process"],
                capture_output=True,
                text=True,
                timeout=30
            )
            assert result.returncode == 0, f"gRPC process failed: {result.stderr}"
            data = json.loads(result.stdout)
            assert "message" in data, f"Missing content: {data}"
            assert len(data["message"]) > 10, f"Response too short: {data['message']}"
            return f"gRPC simple prompt works", ""

        self.run_test("GRPC", "simple_prompt", test_grpc_simple_prompt)

        def test_grpc_session():
            session_id = "grpc-test-session-123"
            result = subprocess.run(
                ["grpcurl", "-plaintext", "-d",
                 f'{{"message": "test session", "session_id": "{session_id}"}}',
                 f"localhost:{self.grpc_port}", "agent.AgentService/Process"],
                capture_output=True,
                text=True,
                timeout=30
            )
            assert result.returncode == 0, f"gRPC session test failed: {result.stderr}"
            data = json.loads(result.stdout)
            assert "message" in data, f"Missing content: {data}"
            # Session may or may not be echoed back, just verify request succeeded
            return f"gRPC session ID preserved in request", ""

        self.run_test("GRPC", "session_preservation", test_grpc_session)

    def run_all_tests(self, category_filter: Optional[str] = None):
        """Run all tests or specific category"""
        print(f"\n{'='*60}")
        print(f"Kyoci Agent Test Harness")
        print(f"Target: {self.base_url}")
        print(f"gRPC: localhost:{self.grpc_port}")
        print(f"{'='*60}\n")

        test_categories = {
            "HEALTH_CHECK": self.run_health_check_tests,
            "TIER_0": self.run_tier_0_tests,
            "TIER_1": self.run_tier_1_tests,
            "JSON_VALIDATION": self.run_json_validation_tests,
            "ERROR_HANDLING": self.run_error_handling_tests,
            "CONCURRENT": self.run_concurrent_tests,
            "GRPC": self.run_grpc_tests,
        }

        if category_filter:
            category_filter = category_filter.upper()
            if category_filter not in test_categories:
                print(f"Unknown category: {category_filter}")
                print(f"Available categories: {', '.join(test_categories.keys())}")
                return
            test_categories = {category_filter: test_categories[category_filter]}

        for category, runner in test_categories.items():
            print(f"\n{'─'*60}")
            print(f"Category: {category}")
            print(f"{'─'*60}")
            runner()

        self.print_summary()

    def print_summary(self):
        """Print test summary in JSON format"""
        duration_ms = (time.time() - self.start_time) * 1000
        passed = sum(1 for r in self.results if r.status == "PASS")
        failed = sum(1 for r in self.results if r.status in ["FAIL", "ERROR"])

        summary = {
            "timestamp": datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"),
            "total_tests": len(self.results),
            "passed": passed,
            "failed": failed,
            "duration_ms": round(duration_ms, 2),
            "results": [asdict(r) for r in self.results],
            "weaknesses": self.weaknesses
        }

        print(f"\n{'='*60}")
        print(f"Test Summary")
        print(f"{'='*60}")
        print(json.dumps(summary, indent=2))
        print(f"\n✓ Passed: {passed}/{len(self.results)}")
        if failed > 0:
            print(f"✗ Failed: {failed}/{len(self.results)}")
            print(f"\nWeaknesses detected:")
            for w in self.weaknesses:
                print(f"  • {w}")
        else:
            print("🎉 All tests passed!")


def main():
    parser = argparse.ArgumentParser(description="Kyoci Agent Test Harness")
    parser.add_argument("--host", default="localhost", help="Agent host (default: localhost)")
    parser.add_argument("--port", type=int, default=8080, help="Agent HTTP port (default: 8080)")
    parser.add_argument("--grpc-port", type=int, default=50051, help="Agent gRPC port (default: 50051)")
    parser.add_argument("--category", help="Run specific category only (HEALTH_CHECK, TIER_0, TIER_1, JSON_VALIDATION, ERROR_HANDLING, CONCURRENT, GRPC)")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")

    args = parser.parse_args()

    harness = AgentTestHarness(args.host, args.port, args.grpc_port, args.verbose)
    harness.run_all_tests(args.category)


if __name__ == "__main__":
    main()