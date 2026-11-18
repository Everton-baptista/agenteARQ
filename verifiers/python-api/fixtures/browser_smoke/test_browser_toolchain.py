"""A real HTTP/browser smoke test for the verifier, not a product benchmark."""
from __future__ import annotations

import json
import threading
from collections.abc import Iterator
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest
from playwright.sync_api import expect, sync_playwright

PAGE = b"""<!doctype html><html lang="en"><head><title>Verifier browser smoke</title>
<style>:focus-visible{outline:3px solid blue}</style></head><body>
<h1>Verifier browser smoke</h1><form id="record"><label for="name">Name</label>
<input id="name" name="name" required><button type="submit">Save</button></form>
<p role="status" aria-live="polite"></p><script>
document.querySelector('form').addEventListener('submit', function(event) {
  event.preventDefault();
  document.querySelector('[role=status]').textContent =
    'Saved: ' + document.querySelector('#name').value;
});
</script></body></html>"""


class Handler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        self.send_response(200)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(PAGE)))
        self.end_headers()
        self.wfile.write(PAGE)

    def log_message(self, format: str, *args: object) -> None:
        pass


@pytest.fixture
def page_url() -> Iterator[str]:
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        server.server_close()
        worker.join(timeout=2)


def test_headless_browser_keyboard_labels_and_feedback(page_url: str) -> None:
    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(headless=True)
        try:
            page = browser.new_page()
            page.goto(page_url)
            expect(page).to_have_title("Verifier browser smoke")
            expect(page.locator("html")).to_have_attribute("lang", "en")
            expect(page.get_by_role("heading", level=1)).to_have_count(1)
            name = page.get_by_label("Name", exact=True)
            name.focus()
            page.keyboard.type("Browser fixture")
            page.keyboard.press("Enter")
            expect(page.get_by_role("status")).to_have_text("Saved: Browser fixture")
            expect(page.get_by_role("button", name="Save")).to_be_enabled()
            print(json.dumps({"browser": "chromium", "version": browser.version}))
        finally:
            browser.close()
