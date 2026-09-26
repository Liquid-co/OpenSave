"""Tests for the plugin's backend, without Decky Loader or a Deck.

`decky` exists only inside Decky Loader, so a stand-in is installed before
main.py is imported; the daemon is a small HTTP server answering with the
shapes the real one sends (internal/api), pinned by the Go side's
TestDeckyPluginContract.

Run: python -m unittest discover -s opensave-decky-plugin/tests
"""

import asyncio
import json
import logging
import os
import sys
import tempfile
import threading
import types
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

HOME = tempfile.mkdtemp(prefix="opensave-decky-test-")
os.makedirs(os.path.join(HOME, ".opensave"), exist_ok=True)

decky = types.ModuleType("decky")
decky.DECKY_USER_HOME = HOME
decky.logger = logging.getLogger("opensave-decky-test")


async def _emit(*args):
    return None


decky.emit = _emit
sys.modules["decky"] = decky
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import main  # noqa: E402  (after the stand-in, on purpose)


class StubDaemon:
    """Answers "METHOD /path" with (status, body), and records requests."""

    def __init__(self, answers):
        self.answers = answers
        self.seen = []
        stub = self

        class Handler(BaseHTTPRequestHandler):
            def _answer(self):
                length = int(self.headers.get("Content-Length") or 0)
                body = json.loads(self.rfile.read(length) or b"null") if length else None
                stub.seen.append((self.command, self.path, body))
                status, payload = stub.answers.get(f"{self.command} {self.path}", (404, {"error": "no such route"}))
                raw = json.dumps(payload).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            do_GET = do_POST = _answer

            def log_message(self, *args):
                pass

        self.server = HTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        with open(main.ADDR_FILE, "w", encoding="utf-8") as fh:
            fh.write(f"127.0.0.1:{self.server.server_port}")

    def close(self):
        self.server.shutdown()
        self.server.server_close()


def run(coro):
    return asyncio.run(coro)


class SyncAnswers(unittest.TestCase):
    def test_a_game_that_did_not_sync_says_why(self):
        cases = {
            "offline": (409, {"error": "no online peers available", "reason": "offline"}),
            "paused": (409, {"error": "syncing is paused"}),  # a daemon from before reasons
            "held": (409, {"error": "its save files were all deleted here; ...", "reason": "held"}),
            "error": (409, {"error": "disk full", "reason": "error"}),
        }
        for want, answer in cases.items():
            stub = StubDaemon({"POST /api/games/hades/sync": answer})
            try:
                res = run(main.Plugin().sync_game("hades"))
            finally:
                stub.close()
            self.assertFalse(res["success"], want)
            self.assertEqual(res["reason"], want)

    def test_a_game_that_synced_says_what_it_did(self):
        stub = StubDaemon({"POST /api/games/hades/sync": (200, {"results": {"p": {"status": "updated"}}})})
        try:
            res = run(main.Plugin().sync_game("hades"))
        finally:
            stub.close()
        self.assertEqual(res, {"success": True, "outcome": "changed"})

    def test_sync_all_no_longer_says_started_when_nothing_could_sync(self):
        offline = {"status": "error", "error": "no online peers available", "reason": "offline"}
        paused = {"status": "error", "error": "syncing is paused", "reason": "paused"}
        self.assertEqual(
            main.summarize_sync_all({"a": offline, "b": offline})["message"],
            "No other device is online — saves sync when one is",
        )
        self.assertEqual(main.summarize_sync_all({"a": paused})["message"], "Syncing is paused — nothing was synced")
        mixed = main.summarize_sync_all({
            "a": {"p": {"status": "updated"}},
            "b": {"p": {"status": "in_sync"}},
            "c": {"p": {"status": "conflict"}},
            "d": {"status": "skipped", "reason": "autoSync disabled"},
        })
        self.assertTrue(mixed["success"])
        self.assertEqual(mixed["message"], "Synced 2 games: 1 updated, 1 already in sync — 1 needs a decision")
        self.assertFalse(main.summarize_sync_all({"a": {"status": "error", "error": "boom"}})["success"])

    def test_sync_all_end_to_end(self):
        stub = StubDaemon({"POST /api/games/sync-all": (200, {"results": {
            "a": {"status": "error", "error": "no online peers available", "reason": "offline"}}})})
        try:
            res = run(main.Plugin().sync_all())
        finally:
            stub.close()
        self.assertFalse(res["success"])
        self.assertIn("No other device is online", res["message"])


class Decisions(unittest.TestCase):
    def test_pause_resume_and_answers_reach_the_daemon(self):
        stub = StubDaemon({
            "POST /api/sync/pause": (200, {"paused": True}),
            "POST /api/sync/resume": (200, {"paused": False}),
            "POST /api/games/hades/emptied": (200, {}),
            "POST /api/games/hades/resolve-location-conflict": (200, {"accepted": True}),
        })
        try:
            p = main.Plugin()
            self.assertTrue(run(p.pause_sync(60))["success"])
            self.assertTrue(run(p.pause_sync(0))["success"])
            self.assertTrue(run(p.resume_sync())["success"])
            self.assertTrue(run(p.answer_emptied("hades", "restore"))["success"])
            self.assertFalse(run(p.answer_emptied("hades", "shrug"))["success"])
            self.assertTrue(run(p.resolve_location_conflict("hades", "peer-1", "Config", "keep-remote"))["success"])
            self.assertFalse(run(p.resolve_location_conflict("hades", "peer-1", "Config", "merge-branch"))["success"])
        finally:
            stub.close()
        bodies = {(m, path): body for m, path, body in stub.seen}
        self.assertEqual(bodies[("POST", "/api/sync/pause")], {"untilRestart": True})  # the last one
        self.assertEqual(bodies[("POST", "/api/games/hades/emptied")], {"answer": "restore"})
        self.assertEqual(
            bodies[("POST", "/api/games/hades/resolve-location-conflict")],
            {"peerId": "peer-1", "root": "Config", "resolution": "keep-remote"},
        )
        self.assertEqual(sum(1 for m, path, _ in stub.seen if path == "/api/games/hades/emptied"), 1)


class FindingTheDaemon(unittest.TestCase):
    def test_install_sh_location_is_found_without_it_on_path(self):
        home = "/home/deck"
        installed = os.path.join(home, ".local", "bin", "opensave-cli")
        cmd = main.daemon_command(home, which=lambda _: None, is_file=lambda p: p == installed)
        self.assertEqual(cmd, [installed, "daemon"])

    def test_path_first_then_flatpak(self):
        self.assertEqual(
            main.daemon_command("/home/deck", which=lambda n: "/opt/opensave-cli" if n == "opensave-cli" else None,
                                is_file=lambda p: True),
            ["/opt/opensave-cli", "daemon"],
        )
        cmd = main.daemon_command("/home/deck", which=lambda n: "/usr/bin/flatpak" if n == "flatpak" else None,
                                  is_file=lambda p: False)
        self.assertEqual(cmd[:3], ["/usr/bin/flatpak", "run", "--command=opensave-cli"])
        self.assertIsNone(main.daemon_command("/home/deck", which=lambda _: None, is_file=lambda p: False))


if __name__ == "__main__":
    unittest.main()
