#!/usr/bin/env python3
# Copyright (c) 2024 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
# Assumes: Python 3.6+

import argparse
import coloredlogs
import glob
import json
import logging
import os
import shutil

import yaml

from git import Repo

# Configure logging with coloredlogs
coloredlogs.install(level='DEBUG')  # Set the logging level as needed

# Config Constants
SCRIPT_DIR = os.path.join(os.path.dirname(os.path.realpath(__file__)))
ROOT_DIR = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))

def save_yaml(file_path, yaml_data):
    """
    Save YAML data to a file.
    
    Args:
        file_path (str): The path to the YAML file.
        yaml_data (dict): The data to be written to the YAML file.
    """
    with open(file_path, "w") as f:
        yaml.dump(yaml_data, f)

def update_yaml_field(file_path, repo_name, new_sha):
    """
    Update the 'sha' field in a YAML file.
    
    Args:
        file_path (str): The path to the YAML file.
        repo_name (str): The name of the repository to update.
        new_sha (str): The new SHA value to set for the repository.
    """
    with open(file_path, "r") as f:
        yaml_data = yaml.safe_load(f)

    for entry in yaml_data:
        if "repo_name" in entry and entry["repo_name"] == repo_name and "sha" in entry:
            logging.info(f"Updating YAML field for repository: {repo_name}")
            entry["sha"] = new_sha
            break

    save_yaml(file_path, yaml_data)
        
def clone_pipeline_repo(org, repo_name, branch, target_path, pat=None):
    """
    Clone the pipeline repository.
    
    Args:
        org (str): The organization name of the repository.
        repo_name (str): The name of the repository to clone.
        branch (str): The branch to checkout after cloning.
        target_path (str): The directory path to clone the repository into.
    """
    logging.info(f"Cloning repository: {repo_name} from branch: {branch} into path: {target_path}")
    if pat:
        # Construct the URL with the PAT
        clone_url = f"https://{pat}@github.com/{org}/{repo_name}.git"

    else:
        logging.warning("Personal Access Token (PAT) not provided. Cloning without authentication.")
        clone_url = f"https://github.com/{org}/{repo_name}.git"

    repository = Repo.clone_from(clone_url, target_path)
    repository.git.checkout(branch)

def fetch_latest_manifest(dir_path):
    """
    Fetch the latest manifest file from the snapshots directory.
    
    Args:
        dir_path (str): The path to the directory containing manifest files.
        
    Returns:
        str or None: The path to the latest manifest file, or None if no manifest is found.
    """
    manifest = glob.glob(os.path.join(dir_path, "manifest.json"))

    return manifest[-1] if manifest else None

def read_json_file(file_path):
    """
    Read JSON data from a file.
    
    Args:
        file_path (str): The path to the JSON file.
        
    Returns:
        dict: The JSON data read from the file.
    """
    logging.info(f"Reading JSON data from file: {file_path}")
    with open(file_path, "r") as file:
        data = json.load(file)

    logging.info("JSON data read successfully.\n")
    return data

def main():
    logging.basicConfig(level=logging.INFO)
    logging.info("Starting Pipeline Manifest Sha Sync script...")

    parser = argparse.ArgumentParser()
    parser.add_argument("--org", dest="org", default="stolostron", required=False, type=str, help="Organization of the repository")
    parser.add_argument("--repo", dest="repo", required=True, type=str, help="Destination repository of the pipeline manifest")
    parser.add_argument("--branch", dest="branch", required=True, type=str, help="Target branch of the pipeline manifest")
    parser.add_argument("--component", dest="component", type=str, required=False, help="If provided, only this component will be processed")
    parser.add_argument("--config", dest="config", type=str, required=False, help="If provided, this config file will be processed")

    # Parse the command line arguments.
    args = parser.parse_args()
    component = args.component
    config_override = args.config

    # Load configuration
    # Config.yaml holds the configurations for Operator bundle locations to be used
    if config_override:
        root_override_path = os.path.join(ROOT_DIR, config_override)
        script_override_path = os.path.join(SCRIPT_DIR, config_override)

        if os.path.exists(root_override_path):
            config_yaml = root_override_path
        else:
            config_yaml = script_override_path
    else:
        config_yaml = os.path.join(SCRIPT_DIR, "config.yaml")

    logging.info("Loading configuration from: %s" % config_yaml)
    with open(config_yaml, 'r') as f:
        config = yaml.safe_load(f)

    # Clone pipeline repository into temporary directory path.
    repo_directory = os.path.join(os.path.dirname(os.path.realpath(__file__)), f"tmp/{args.repo}")
    if os.path.exists(repo_directory): # If path exists, remove and re-clone
        logging.warning("The repository directory already exists, removing directory at: %s" % repo_directory)
        shutil.rmtree(repo_directory)
    logging.info("Cloning pipeline repository: %s/%s (branch: %s)" % (args.org, args.repo, args.branch))
    clone_pipeline_repo(args.org, args.repo, args.branch, target_path=repo_directory, pat=os.getenv("GH_READ_PAT"))

    # Fetch latest manifest
    snapshots_path = os.path.join(repo_directory, ".")
    if not os.path.exists(snapshots_path):
        logging.error("Snapshots directory does not exist in repository: %s" % args.repo)
        exit(1)

    logging.info("Fetching latest manifest from snapshots directory")
    manifest_file_path = fetch_latest_manifest(snapshots_path)
    if not manifest_file_path:
        logging.error("Failed to fetch latest manifest file from snapshots in repository: %s" % args.repo)
        exit(1)
    logging.info("Latest manifest file fetched successfully from snapshots in repository: %s" % args.repo)

    # Read manifest data
    logging.info("Reading manifest data from file: %s" % manifest_file_path)
    manifest_data = read_json_file(manifest_file_path)

    # Normalize config into a list of components
    if isinstance(config, dict):
        components = config.get("components", [])
    else:
        components = config

    # Optionally filter by a specific component
    if component:
        components = [repo for repo in components if repo.get("repo_name") == component]

    for repo in components:
        if "sha" in repo and "repo_name" in repo:
            logging.info("Checking repository for updates: %s" % repo.get("repo_name"))
            found_match = False

            # Compare git-sha256 values
            for entry in manifest_data:
                if entry.get("image-name") == repo.get("repo_name") and entry.get("git-sha256") != repo.get("sha"):
                    found_match = True
                    logging.warning(f"Sha mismatch for repository {repo.get('repo_name')}: YAML sha {repo.get('sha')}, JSON sha {entry.get('git-sha256')}")
                    update_yaml_field(config_yaml, repo.get("repo_name"), entry.get("git-sha256"))
                    break

            if not found_match:
                logging.info(f"No SHA mismatch found for repository {repo.get('repo_name')}")

            print("\n")
            
    logging.info("All repositories and operators processed successfully.")
    logging.info("Performing cleanup...")
    shutil.rmtree((os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp")), ignore_errors=True)

    logging.info("Cleanup completed.")
    logging.info("Script execution completed.")

if __name__ == '__main__':
    main()
