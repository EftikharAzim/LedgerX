#!/usr/bin/env python3
"""Check the real smoke target's exit status and cleanup without starting services."""

import errno
from pathlib import Path
import shlex
import socket
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class SmokeTargetTests(unittest.TestCase):
    def run_target(self, smoke_exit=0, start_exit=0):
        with tempfile.TemporaryDirectory(prefix="ledgerx-smoke-target-") as directory:
            fixture = Path(directory)
            overlay = fixture / "stub.mk"
            overlay.write_text(
                ".PHONY: up migrate-up api-start api-stop smoke\n"
                "up migrate-up:\n\t@true\n"
                "api-start:\n\t@exit $(SMOKE_TEST_START_EXIT)\n"
                'api-stop:\n\t@touch "$(SMOKE_TEST_STOP_MARKER)"\n'
                'smoke:\n\t@touch "$(SMOKE_TEST_RUN_MARKER)"; exit $(SMOKE_TEST_EXIT)\n'
            )
            stop_marker = fixture / "stopped"
            run_marker = fixture / "ran"
            command = ["make", "--no-print-directory", "-f", str(ROOT / "Makefile"), "-f", str(overlay)]
            # The target's readiness check only needs a listening socket. If a
            # local DB already owns the port, leave that service untouched.
            listener = socket.socket()
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            try:
                listener.bind(("127.0.0.1", 5432))
                listener.listen()
            except OSError as error:
                listener.close()
                listener = None
                if error.errno != errno.EADDRINUSE:
                    raise
            try:
                result = subprocess.run(
                    command + [
                        "e2e-smoke", f"MAKE={shlex.join(command)}",
                        f"SMOKE_TEST_EXIT={smoke_exit}",
                        f"SMOKE_TEST_START_EXIT={start_exit}",
                        f"SMOKE_TEST_STOP_MARKER={stop_marker}",
                        f"SMOKE_TEST_RUN_MARKER={run_marker}",
                    ],
                    cwd=ROOT, capture_output=True, text=True, timeout=20, check=False,
                )
            finally:
                if listener is not None:
                    listener.close()
            return result, stop_marker.exists(), run_marker.exists()

    def test_failure_is_returned_and_api_is_stopped(self):
        result, stopped, ran = self.run_target(smoke_exit=1)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(ran)
        self.assertTrue(stopped)

    def test_success_is_returned_and_api_is_stopped(self):
        result, stopped, ran = self.run_target()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(ran)
        self.assertTrue(stopped)

    def test_start_failure_still_stops_api_without_running_smoke(self):
        result, stopped, ran = self.run_target(start_exit=1)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(ran)
        self.assertTrue(stopped)


if __name__ == "__main__":
    unittest.main()
