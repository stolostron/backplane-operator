#!/usr/bin/env python3
"""
Utility module for fetching git commit SHAs from remote repositories.
"""

import re
import logging
from git import Git


def fetch_sha_from_git_remote(git_url, branch):
    """
    Fetches the latest commit SHA for a branch from a Git repository using ls-remote.

    Args:
        git_url (str): Git repository URL (e.g., https://github.com/openshift/hive.git)
        branch (str): Branch name to fetch SHA for (e.g., master)

    Returns:
        str: The commit SHA for the specified branch

    Raises:
        Exception: If git ls-remote fails or SHA cannot be extracted
    """
    try:
        logging.info(f"Fetching latest SHA from {git_url} branch {branch}")
        g = Git()
        # ls-remote returns lines like: "<sha>\trefs/heads/<branch>"
        output = g.ls_remote(git_url, f"refs/heads/{branch}")

        if not output:
            raise ValueError(f"No output from git ls-remote for {git_url} branch {branch}")

        # Extract SHA from the first line (format: "sha\trefs/heads/branch")
        sha = output.split()[0]

        # Validate it looks like a SHA (40 hex chars for full SHA)
        if not re.match(r'^[a-f0-9]{40}$', sha):
            raise ValueError(f"Invalid SHA format from git ls-remote: {sha}")

        logging.info(f"Successfully fetched SHA: {sha}")
        return sha
    except Exception as e:
        logging.error(f"Failed to fetch SHA from {git_url} branch {branch}: {e}")
        raise
