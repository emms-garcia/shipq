import pathlib
import tempfile
import unittest

from scripts import check_core_coverage


class CoreCoverageCheckTests(unittest.TestCase):
    def test_counts_statements_and_enforces_threshold(self):
        with tempfile.TemporaryDirectory() as directory:
            profile = pathlib.Path(directory) / "coverage.out"
            profile.write_text(
                "mode: atomic\n"
                "example/a.go:1.1,2.1 90 1\n"
                "example/a.go:3.1,4.1 10 0\n"
            )
            result = check_core_coverage.read_profile(profile)
            self.assertEqual(result.total, 100)
            self.assertEqual(result.covered, 90)
            self.assertEqual(result.percent, 90.0)

    def test_merges_duplicate_blocks_from_multiple_package_test_binaries(self):
        with tempfile.TemporaryDirectory() as directory:
            profile = pathlib.Path(directory) / "coverage.out"
            profile.write_text(
                "mode: atomic\n"
                "example/a.go:1.1,2.1 10 0\n"
                "example/a.go:1.1,2.1 10 1\n"
            )
            result = check_core_coverage.read_profile(profile)
            self.assertEqual((result.covered, result.total), (10, 10))

    def test_empty_and_under_threshold_profiles_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            empty = pathlib.Path(directory) / "empty.out"
            empty.write_text("mode: atomic\n")
            with self.assertRaises(ValueError):
                check_core_coverage.read_profile(empty)
            low = pathlib.Path(directory) / "low.out"
            low.write_text("mode: atomic\nexample/a.go:1.1,2.1 9 1\nexample/a.go:3.1,4.1 2 0\n")
            self.assertLess(check_core_coverage.read_profile(low).percent, 90.0)


if __name__ == "__main__":
    unittest.main()
