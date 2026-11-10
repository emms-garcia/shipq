import json
import re
import unittest
from pathlib import Path
from urllib.parse import parse_qs, urlparse


ROOT = Path(__file__).resolve().parent


def load(relative_path):
    return json.loads((ROOT / relative_path).read_text())


class FixtureIntegrityTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.prs = load("github/pr_pages.json")
        cls.reviews = load("github/reviews.json")
        cls.scenarios = load("scenarios/pr_management.json")
        cls.pr_by_id = {
            pr["node_id"]: pr
            for page in cls.prs["pages"]
            for pr in page["response"]["body"]
        }

    def test_remote_pages_and_reviews_reference_known_records(self):
        organizations = {org["node_id"] for org in self.prs["organizations"]}
        repositories = {repo["node_id"]: repo for repo in self.prs["repositories"]}
        for repo in repositories.values():
            self.assertIn(repo["organization_node_id"], organizations)
        for page in self.prs["pages"]:
            for pr in page["response"]["body"]:
                with self.subTest(pr=pr["node_id"]):
                    repo = repositories[pr["base"]["repo"]["node_id"]]
                    self.assertEqual(repo["full_name"], pr["base"]["repo"]["full_name"])
                    self.assertEqual(
                        page["request"]["path"], f'/repos/{repo["full_name"]}/pulls'
                    )
        for page in self.reviews["pages"] + self.reviews["failures"]:
            with self.subTest(page=page["id"]):
                pr = self.pr_by_id[page["pr_node_id"]]
                repo_name = pr["base"]["repo"]["full_name"]
                self.assertEqual(
                    page["request"]["path"],
                    f'/repos/{repo_name}/pulls/{pr["number"]}/reviews',
                )
                if page["response"]["status"] == 200:
                    for review in page["response"]["body"]:
                        self.assertRegex(review["commit_id"], r"^[0-9a-f]{40}$")
                        if review["state"] == "APPROVED":
                            self.assertNotEqual(
                                review["user"]["node_id"], pr["user"]["node_id"]
                            )

    def test_next_links_resolve_to_supplied_success_or_failure_pages(self):
        for dataset in (self.prs, self.reviews):
            pages = dataset["pages"] + dataset["failures"]
            self.assertEqual(len(pages), len({page["id"] for page in pages}))
            for page in pages:
                link = page["response"]["headers"].get("Link")
                if not link:
                    continue
                with self.subTest(page=page["id"]):
                    match = re.fullmatch(r'<([^>]+)>; rel="next"', link)
                    self.assertIsNotNone(match)
                    url = urlparse(match.group(1))
                    query = parse_qs(url.query)
                    self.assertTrue(
                        any(
                            candidate["request"]["path"] == url.path
                            and {
                                key: [str(value)]
                                for key, value in candidate["request"]["query"].items()
                            } == query
                            for candidate in pages
                        ),
                        f"Missing page for {link}",
                    )

    def test_scenario_references_resolve_to_fixture_inputs(self):
        known_pages = {
            "pr_pages": {
                page["id"] for page in self.prs["pages"] + self.prs["failures"]
            },
            "review_pages": {
                page["id"]
                for page in self.reviews["pages"] + self.reviews["failures"]
            },
        }
        inputs = [self.scenarios["initial"]]
        for scenario in self.scenarios["scenarios"]:
            if "pr_node_id" in scenario:
                self.assertIn(scenario["pr_node_id"], self.pr_by_id)
            inputs.extend(scenario["steps"])
        for entry in inputs:
            for key, ids in known_pages.items():
                for reference in entry.get(key, []):
                    self.assertIn(reference, ids)
        for listing in self.scenarios["listings"]:
            for reference in listing["matches"]:
                self.assertIn(reference, self.pr_by_id)
        for reference in self.reviews["expected_effective_approvers"]:
            self.assertIn(reference, self.pr_by_id)

    def test_protocol_examples_do_not_expose_host_owned_credentials(self):
        def check(value):
            if isinstance(value, dict):
                forbidden = {"access_token", "refresh_token", "device_code", "client_secret"}
                self.assertTrue(forbidden.isdisjoint(value))
                for child in value.values():
                    check(child)
            elif isinstance(value, list):
                for child in value:
                    check(child)

        check(load("protocol/base_envelopes.json"))


if __name__ == "__main__":
    unittest.main()
