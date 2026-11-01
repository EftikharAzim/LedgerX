import csv
import tempfile
import unittest
from pathlib import Path

from summarize_customer_tests import observations, report


class CustomerEvidenceTests(unittest.TestCase):
    def read(self, rows):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "results.csv"
            with path.open("w", newline="") as stream:
                writer = csv.DictWriter(stream, fieldnames=["participant_id", "task_id", "outcome", "duration_seconds", "assistance_count"])
                writer.writeheader()
                writer.writerows(rows)
            return observations(path)

    def test_blank_template_is_not_customer_evidence(self):
        rows = self.read([{"participant_id": "P1", "task_id": "T1", "outcome": ""}])
        self.assertEqual(rows, [])
        self.assertIn("Observed participants: **0**", report(rows))
        self.assertIn("No customer sessions", report(rows))

    def test_attempts_assistance_and_skips_are_separate(self):
        rows = self.read([
            {"participant_id": "P1", "task_id": "T1", "outcome": "unaided", "duration_seconds": "40"},
            {"participant_id": "P2", "task_id": "T1", "outcome": "assisted", "duration_seconds": "80", "assistance_count": "1"},
            {"participant_id": "P3", "task_id": "T1", "outcome": "failed"},
            {"participant_id": "P4", "task_id": "T1", "outcome": "skipped"},
        ])
        self.assertIn("| 3 | 1 | 1 | 1 | 1 | 60 (n=2) |", report(rows))

    def test_duplicate_and_misreported_unaided_rows_are_rejected(self):
        row = {"participant_id": "P1", "task_id": "T1", "outcome": "unaided"}
        with self.assertRaises(ValueError):
            self.read([row, row])
        with self.assertRaises(ValueError):
            self.read([{**row, "assistance_count": "1"}])
        with self.assertRaises(ValueError):
            self.read([{**row, "duration_seconds": "NaN"}])


if __name__ == "__main__":
    unittest.main()
