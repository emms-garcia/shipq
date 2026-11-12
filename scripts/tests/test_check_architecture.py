import pathlib
import tempfile
import unittest

from scripts import check_architecture


class ArchitectureCheckTests(unittest.TestCase):
    def test_accepts_inward_imports_and_ignores_tests(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            self.write(root, "internal/domain/value.go", 'package domain\nimport "time"\n')
            self.write(
                root,
                "internal/application/usecase.go",
                'package application\nimport "example.test/project/internal/domain"\n',
            )
            self.write(
                root,
                "internal/domain/value_test.go",
                'package domain\nimport "example.test/project/internal/adapters/sqlite"\n',
            )
            self.assertEqual(check_architecture.find_violations(root), [])

    def test_reports_outward_domain_and_application_imports(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            self.write(
                root,
                "internal/domain/value.go",
                'package domain\nimport "example.test/project/internal/application"\n',
            )
            self.write(
                root,
                "internal/application/usecase.go",
                'package application\nimport (\n "example.test/project/internal/adapters/sqlite"\n)\n',
            )
            violations = check_architecture.find_violations(root)
            self.assertEqual(len(violations), 2)
            report = "\n".join(violations)
            self.assertIn("internal/application/usecase.go", report)
            self.assertIn("internal/adapters/sqlite", report)
            self.assertIn("internal/domain/value.go", report)

    @staticmethod
    def write(root, relative, content):
        path = root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)


if __name__ == "__main__":
    unittest.main()
