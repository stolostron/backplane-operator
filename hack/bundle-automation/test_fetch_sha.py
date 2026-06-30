#!/usr/bin/env python3
"""
Unit tests for the fetch_sha_from_git_remote function.
"""

import unittest
from utils.git_sha_fetcher import fetch_sha_from_git_remote


class TestFetchShaFromGitRemote(unittest.TestCase):
    """Test cases for fetch_sha_from_git_remote function."""

    def test_fetch_sha_from_hive_master(self):
        """Test fetching SHA from openshift/hive master branch."""
        git_url = "https://github.com/openshift/hive.git"
        branch = "master"

        sha = fetch_sha_from_git_remote(git_url, branch)

        # Verify SHA is a valid 40-character hex string
        self.assertIsNotNone(sha)
        self.assertEqual(len(sha), 40)
        self.assertRegex(sha, r'^[a-f0-9]{40}$')
        print(f"\n✓ Successfully fetched SHA from {git_url} branch {branch}: {sha}")

    def test_invalid_branch_raises_error(self):
        """Test that an invalid branch raises an appropriate error."""
        git_url = "https://github.com/openshift/hive.git"
        branch = "this-branch-does-not-exist-12345"

        with self.assertRaises(ValueError) as context:
            fetch_sha_from_git_remote(git_url, branch)

        self.assertIn("No output from git ls-remote", str(context.exception))
        print(f"\n✓ Correctly raised error for invalid branch")

    def test_invalid_repo_raises_error(self):
        """Test that an invalid repository raises an appropriate error."""
        git_url = "https://github.com/nonexistent/repository-12345.git"
        branch = "master"

        with self.assertRaises(Exception):
            fetch_sha_from_git_remote(git_url, branch)

        print(f"\n✓ Correctly raised error for invalid repository")


if __name__ == '__main__':
    # Run tests with verbose output
    unittest.main(verbosity=2)
