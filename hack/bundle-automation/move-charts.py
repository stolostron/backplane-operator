#!/usr/bin/env python3
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
# Assumes: Python 3.6+

import argparse
import os
import re
import shutil
import yaml
import logging
import subprocess
from git import Repo, exc
from packaging import version

from validate_csv import *

# Config Constants
SCRIPT_DIR = os.path.join(os.path.dirname(os.path.realpath(__file__)))
ROOT_DIR = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))

def is_version_compatible(branch, min_release_version, min_backplane_version, min_ocm_version, enforce_master_check=True):
    """
    Check if the current version meets the minimum required version for a feature.

    Args:
        min_release_version (_type_): _description_
        min_backplane_version (_type_): _description_
        min_ocm_version (_type_): _description_
        enforce_master_check (bool, optional): _description_. Defaults to True.

    Returns:
        _type_: _description_
    """
    # Retrieve the release versions from environment variables
    acm_release_version = os.getenv('ACM_RELEASE_VERSION')
    mce_release_version = os.getenv('MCE_RELEASE_VERSION')

    if not acm_release_version and not mce_release_version:
        # logging.error("Neither ACM nor MCE release version is set in environment variables.")
        # If no env vars set, try to determine version from branch
        # Extract the version part from the branch name (e.g., '2.12-integration' -> '2.12')
        pattern = r'(\d+\.\d+)'  # Matches versions like '2.12'

        # Special handling for master branch
        if enforce_master_check and branch in ['main', 'master']:
            logging.info("Branch is 'main' or 'master', assuming latest version compatible with all features.")
            return True

        match = re.search(pattern, branch)
        if match:
            v = match.group(1)  # Extract the version
            branch_version = version.Version(v)  # Create a Version object

            if 'ocm' in branch.lower():
                min_branch_version = version.Version(min_ocm_version)  # Use the minimum release version
            elif 'backplane' in branch.lower():
                min_branch_version = version.Version(min_release_version)  # Use the minimum release version
            else:
                min_branch_version = version.Version(min_backplane_version)  # Use the minimum backplane version

            logging.debug(f"Branch version: {branch_version}, Min version: {min_branch_version}")
            logging.debug(f"Is compatible: {branch_version >= min_branch_version}")

            # Check if the branch version is compatible with the specified minimum branch
            return branch_version >= min_branch_version
        else:
            logging.error("Version not found in branch: %s", branch)
            return False

    if acm_release_version and version.Version(acm_release_version) >= version.Version(min_release_version):
        return True

    elif mce_release_version and version.Version(mce_release_version) >= version.Version(min_backplane_version):
        return True

    return False

def inject_probe_config_helm_templates(deployment_file):
    """
    Injects conditional Helm templates for probe configuration into deployment files.

    Adds template blocks for exec probes that allow users to optionally configure:
    - timeoutSeconds (K8s default: 1) - all probe types
    - failureThreshold (K8s default: 3) - all probe types
    - successThreshold (K8s default: 1) - readinessProbe only

    Note: successThreshold is only injected for readinessProbe because Kubernetes
    validation requires livenessProbe.successThreshold to be exactly 1, making it
    non-configurable for liveness and startup probes.

    This addresses ACM-29011 where customers on compact clusters or slow I/O systems
    experience frequent probe timeouts. Rather than forcing new defaults on all users,
    this allows opt-in configuration via MCH CR probeConfig field while preserving
    Kubernetes defaults when not configured.

    Only applies to exec probes (not httpGet/tcpSocket). Only injects if values don't
    already exist in the template.

    Note: This function is only called for ACM 2.17+ and MCE 2.17+ releases.

    Args:
        deployment_file (str): Path to the deployment YAML file to update
    """
    with open(deployment_file, 'r', encoding='utf-8') as f:
        lines = f.readlines()

    i = 0
    while i < len(lines):
        line = lines[i]

        # Find probe sections
        probe_match = re.match(r'^(\s+)(livenessProbe|readinessProbe|startupProbe):\s*$', line)
        if probe_match:
            base_indent = len(probe_match.group(1))
            probe_type = probe_match.group(2)
            field_indent = ' ' * (base_indent + 2)

            # Collect the probe section
            probe_section = []
            j = i + 1
            while j < len(lines) and lines[j].strip():
                line_indent = len(lines[j]) - len(lines[j].lstrip())
                if line_indent <= base_indent:
                    break
                probe_section.append(lines[j])
                j += 1

            probe_text = ''.join(probe_section)

            # Only process exec probes
            if 'exec:' in probe_text:
                # Append missing fields
                if 'timeoutSeconds:' not in probe_text:
                    lines.insert(j, '{{- if and .Values.hubconfig.probeConfig (hasKey .Values.hubconfig.probeConfig "timeoutSeconds") }}\n')
                    lines.insert(j+1, f'{field_indent}timeoutSeconds: {{{{ .Values.hubconfig.probeConfig.timeoutSeconds }}}}\n')
                    lines.insert(j+2, '{{- end }}\n')
                    j += 3

                if 'failureThreshold:' not in probe_text:
                    lines.insert(j, '{{- if and .Values.hubconfig.probeConfig (hasKey .Values.hubconfig.probeConfig "failureThreshold") }}\n')
                    lines.insert(j+1, f'{field_indent}failureThreshold: {{{{ .Values.hubconfig.probeConfig.failureThreshold }}}}\n')
                    lines.insert(j+2, '{{- end }}\n')
                    j += 3

                # Only inject successThreshold for readinessProbe
                # livenessProbe.successThreshold must be 1 (K8s validation requirement)
                if probe_type == 'readinessProbe' and 'successThreshold:' not in probe_text:
                    lines.insert(j, '{{- if and .Values.hubconfig.probeConfig (hasKey .Values.hubconfig.probeConfig "successThreshold") }}\n')
                    lines.insert(j+1, f'{field_indent}successThreshold: {{{{ .Values.hubconfig.probeConfig.successThreshold }}}}\n')
                    lines.insert(j+2, '{{- end }}\n')
                    j += 3

            i = j
        else:
            i += 1

    with open(deployment_file, 'w', encoding='utf-8') as f:
        f.writelines(lines)

def fix_probe_config_templates(destination_template_dir):
    """
    Inject probeConfig templates into deployment files for ACM/MCE 2.17+.

    Processes all deployment YAML files in the destination template directory
    and injects conditional probe configuration templates.
    """
    if not os.path.exists(destination_template_dir):
        logging.warning(f"Template directory does not exist: {destination_template_dir}")
        return

    for filename in os.listdir(destination_template_dir):
        if not filename.endswith('.yaml'):
            continue

        filepath = os.path.join(destination_template_dir, filename)

        try:
            inject_probe_config_helm_templates(filepath)
            logging.debug(f"Injected probeConfig templates in: {filename}")
        except Exception as e:
            logging.error(f"Error processing {filepath}: {e}")

# Copy chart-templates to a new helmchart directory
def copyHelmChart(destinationChartPath, repo, chart, branch):
    chartName = chart['name']
    logging.info(f"Copying templates into new {chartName} chart directory ...")

    # Create main folder
    chartPath = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp", repo, chart["chart-path"])
    if os.path.exists(destinationChartPath):
        logging.info(f"Removing existing directory at: {destinationChartPath}")
        shutil.rmtree(destinationChartPath)

    # Copy Chart.yaml, values.yaml, and templates dir
    chartTemplatesPath = os.path.join(chartPath, "templates/")
    destinationTemplateDir = os.path.join(destinationChartPath, "templates/")
    os.makedirs(destinationTemplateDir)
    logging.debug(f"Created destination template directory at: {destinationTemplateDir}")

    # Fetch template files
    logging.info(f"Copying template files from '{chartTemplatesPath}' to '{destinationTemplateDir}'")
    for file_name in os.listdir(chartTemplatesPath):
        # Construct full file path
        source = os.path.join(chartTemplatesPath, file_name)
        destination = os.path.join(destinationTemplateDir, file_name)

        # Copy only files
        if os.path.isfile(source):
            logging.debug(f"Copying file '{source}' to '{destination}'")
            shutil.copyfile(source, destination)
        else:
            logging.warning(f"Skipping non-file item: {source}")

    chartYamlPath = os.path.join(chartPath, "Chart.yaml")
    if not os.path.exists(chartYamlPath):
        logging.error(f"No Chart.yaml found for chart: '{chartName}'")
        return

    logging.info("Copying Chart.yaml to '%s'", os.path.join(destinationChartPath, "Chart.yaml"))
    shutil.copyfile(chartYamlPath, os.path.join(destinationChartPath, "Chart.yaml"))

    valuesYamlPath = os.path.join(chartPath, "values.yaml")
    if not os.path.exists(valuesYamlPath):
        logging.error(f"No values.yaml found for chart: '{chartName}'")
        return

    destinationValuesPath = os.path.join(destinationChartPath, "values.yaml")
    shutil.copyfile(valuesYamlPath, destinationValuesPath)

    # Inject probeConfig into values.yaml if not present (ACM/MCE 2.17+)
    if is_version_compatible(branch, '2.17', '2.17', '2.17'):
        try:
            with open(destinationValuesPath, 'r') as f:
                content = f.read()

            values = yaml.safe_load(content)

            if 'hubconfig' in values and 'probeConfig' not in values['hubconfig']:
                # Insert probeConfig after ocpVersion to maintain field order
                lines = content.split('\n')
                for i, line in enumerate(lines):
                    if line.strip().startswith('ocpVersion:'):
                        # Insert probeConfig on next line with same indentation
                        indent = len(line) - len(line.lstrip())
                        lines.insert(i + 1, ' ' * indent + 'probeConfig: null')
                        break

                with open(destinationValuesPath, 'w') as f:
                    f.write('\n'.join(lines))
                logging.info("Added probeConfig to values.yaml")
        except Exception as e:
            logging.error(f"Error injecting probeConfig into values.yaml: {e}")

    logging.info("Chart copied.\n")

    # Fix probeConfig template checks (ACM/MCE 2.17+)
    if is_version_compatible(branch, '2.17', '2.17', '2.17'):
        fix_probe_config_templates(destinationTemplateDir)

def addCRDs(repo, chart, outputDir):
    if not 'chart-path' in chart:
        logging.critical("Could not validate chart path in given chart: " + chart)
        exit(1)

    chartPath = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp", repo, chart["chart-path"])
    if not os.path.exists(chartPath):
        logging.critical("Could not validate chartPath at given path: " + chartPath)
        exit(1)

    # Use custom crd-path if specified (from repo root), otherwise default to chart-path/crds
    repoPath = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp", repo)
    if "crd-path" in chart:
        crdPath = os.path.join(repoPath, chart["crd-path"])
        logging.info(f"Using custom CRD path: {chart['crd-path']}")
    else:
        crdPath = os.path.join(chartPath, "crds")
        logging.info(f"Using default CRD path: {os.path.join(chart["chart-path"], "crds")}")

    if not os.path.exists(crdPath):
        logging.info(f"No CRDs for repo: {repo} at path: {crdPath}")
        return

    destinationPath = os.path.join(outputDir, "crds", chart['name'])
    if os.path.exists(destinationPath): # If path exists, remove and re-clone
        shutil.rmtree(destinationPath)
    os.makedirs(destinationPath)
    for filename in os.listdir(crdPath):
        if not filename.endswith(".yaml"): 
            continue
        filepath = os.path.join(crdPath, filename)
        with open(filepath, 'r') as f:
            resourceFile = yaml.safe_load(f)

        if resourceFile["kind"] == "CustomResourceDefinition":
            shutil.copyfile(filepath, os.path.join(destinationPath, filename))

def chartConfigAcceptable(chart):
    helmChart = chart["name"]
    if helmChart == "":
        logging.critical("Unable to generate helm chart without a name.")
        return False
    return True

def main():
    ## Initialize ArgParser
    parser = argparse.ArgumentParser()
    parser.add_argument("--component", dest="component", type=str, required=False, help="If provided, only this component will be processed")
    parser.add_argument("--config", dest="config", type=str, required=False, help="If provided, this config file will be processed")
    parser.add_argument("--destination", dest="destination", type=str, required=False, help="Destination directory of the created helm chart")

    args = parser.parse_args()
    component = args.component
    config_override = args.config
    destination = args.destination

    logging.basicConfig(level=logging.DEBUG)

    # Config.yaml holds the configurations for Operator bundle locations to be used
    if config_override:
        root_override_path = os.path.join(ROOT_DIR, config_override)
        script_override_path = os.path.join(SCRIPT_DIR, config_override)

        if os.path.exists(root_override_path):
            config_yaml = root_override_path
        else:
            config_yaml = script_override_path
    else:
        config_yaml = os.path.join(SCRIPT_DIR, "copy-config.yaml")

    with open(config_yaml, 'r') as f:
        config = yaml.safe_load(f)

    # Set release version environment variables if provided in config
    if isinstance(config, dict):
        os.environ['ACM_RELEASE_VERSION'] = config.get('acm-release-version', '')
        os.environ['MCE_RELEASE_VERSION'] = config.get('mce-release-version', '')

    # Normalize config into a list of components
    if isinstance(config, dict):
        components = config.get("components", [])
    else:
        components = config

    # Optionally filter by a specific component
    if component:
        components = [repo for repo in components if repo.get("repo_name") == component]

    # Loop through each repo in the config.yaml
    for repo in components:
        logging.info("Cloning: %s", repo["repo_name"])
        repo_path = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp/" + repo["repo_name"]) # Path to clone repo to
        if os.path.exists(repo_path): # If path exists, remove and re-clone
            shutil.rmtree(repo_path)

        repository = Repo.clone_from(repo["github_ref"], repo_path) # Clone repo to above path
        branch = repo.get('branch', 'main')  # Default to 'main' if no branch specified
        if 'branch' in repo:
            repository.git.checkout(repo['branch']) # If a branch is specified, checkout that branch

        # Loop through each operator in the repo identified by the config
        for chart in repo["charts"]:
            if not chartConfigAcceptable(chart):
                logging.critical("Unable to generate helm chart without configuration requirements.")
                exit(1)

            logging.info("Helm Chartifying -  %s!\n", chart["name"])

            logging.info("Adding CRDs -  %s!\n", chart["name"])
            # Copy over all CRDs to the destination directory
            addCRDs(repo["repo_name"], chart, destination)

            logging.info("Creating helm chart: '%s' ...", chart["name"])

            always_or_toggle = chart['always-or-toggle']
            destinationChartPath = os.path.join(destination, "charts", always_or_toggle, chart['name'])

            # Template Helm Chart Directory from 'chart-templates'
            logging.info("Templating helm chart '%s' ...", chart["name"])
            copyHelmChart(destinationChartPath, repo["repo_name"], chart, branch)

    logging.info("All repositories and operators processed successfully.")
    logging.info("Performing cleanup...")
    shutil.rmtree((os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp")), ignore_errors=True)

    logging.info("Cleanup completed.")
    logging.info("Script execution completed.")

if __name__ == "__main__":
   main()
