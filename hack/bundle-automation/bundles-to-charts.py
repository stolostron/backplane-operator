#!/usr/bin/env python3
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
# Assumes: Python 3.6+

"""_summary_

Raises:
    ValueError: _description_

Returns:
    _type_: _description_
"""

import argparse
import os
import shutil
import logging
import re
import sys
import tempfile
import coloredlogs
import yaml

from git import Repo, Git
from packaging import version
from validate_csv import *
from utils.git_sha_fetcher import fetch_sha_from_git_remote

# Configure logging with coloredlogs
coloredlogs.install(level='DEBUG')  # Set the logging level as needed

# Removed literal_str class - using post-processing instead

# Config Constants
SCRIPT_DIR = os.path.join(os.path.dirname(os.path.realpath(__file__)))
ROOT_DIR = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))

def log_header(message, *args):
    """
    Logs a header message with visual separators and formats the message using multiple arguments.
    Args:
        message (str): The message to be displayed as the header.
        *args: Additional arguments to be passed into the message string.
    """
    # Format the message with the provided arguments
    formatted_message = message.format(*args)

    # Create a separator line that matches the length of the formatted message
    separator = "=" * len(formatted_message)

    # Log an empty line before the separator and the header
    logging.info("")

    # Log the separator, the formatted message, and the separator again
    logging.info(separator)
    logging.info(formatted_message)
    logging.info(separator)

# Split a string at a specified delimiter.  If delimiter doesn't exist, consider the
# string to be all "left-part" (before delimiter) or "right-part" as requested.
def split_at(the_str, the_delim, favor_right=True):
    """_summary_

    Args:
        the_str (_type_): _description_
        the_delim (_type_): _description_
        favor_right (bool, optional): _description_. Defaults to True.

    Returns:
        _type_: _description_
    """
    split_pos = the_str.find(the_delim)
    if split_pos > 0:
        left_part  = the_str[0:split_pos]
        right_part = the_str[split_pos+1:]
    else:
        if favor_right:
            left_part  = None
            right_part = the_str
        else:
            left_part  = the_str
            right_part = None

    return (left_part, right_part)

# Parse an image reference, return dict containing image reference information
def parse_image_ref(image_ref):
    """_summary_

    Args:
        image_ref (_type_): _description_

    Returns:
        _type_: _description_
    """
    # Image ref:  [registry-and-ns/]repository-name[:tag][@digest]
    parsed_ref = dict()

    remaining_ref = image_ref
    at_pos = remaining_ref.rfind("@")
    if at_pos > 0:
        parsed_ref["digest"] = remaining_ref[at_pos+1:]
        remaining_ref = remaining_ref[0:at_pos]
    else:
        parsed_ref["digest"] = None
    colon_pos = remaining_ref.rfind(":")
    if colon_pos > 0:
        parsed_ref["tag"] = remaining_ref[colon_pos+1:]
        remaining_ref = remaining_ref[0:colon_pos]
    else:
        parsed_ref["tag"] = None
    slash_pos = remaining_ref.rfind("/")
    if slash_pos > 0:
        parsed_ref["repository"] = remaining_ref[slash_pos+1:]
        rgy_and_ns = remaining_ref[0:slash_pos]
    else:
        parsed_ref["repository"] = remaining_ref
        rgy_and_ns = "localhost"
    parsed_ref["registry_and_namespace"] = rgy_and_ns

    rgy, ns = split_at(rgy_and_ns, "/", favor_right=False)
    if not ns:
        ns = ""

    parsed_ref["registry"] = rgy
    parsed_ref["namespace"] = ns

    slash_pos = image_ref.rfind("/")
    if slash_pos > 0:
        repo_and_suffix = image_ref[slash_pos+1:]
    else:
        repo_and_suffix = image_ref
    parsed_ref["repository_and_suffix"]  = repo_and_suffix

    return parsed_ref

def lookup_image_key(repository, image_key_mapping):
    return image_key_mapping.get(repository)

def format_image_mapping_error(operator_name, file_path, field, repository):
    return (
        f"[{operator_name}] No image key mapping for repository '{repository}' "
        f"in {field} ({file_path})"
    )

# Copy chart-templates to a new helmchart directory
def templateHelmChart(outputDir, helmChart, preservedFiles=None, overwrite=False):
    """
    Copies templates into a new helm chart directory.

    Args:
        outputDir (str): The directory where the helm chart will be created.
        helmChart (str): The name of the new helm chart directory.
        preservedFiles (list, optional): List of filenames to preserve. Defaults to None.
        overwrite (bool, optional): Whether to overwrite existing files. Defaults to False.
    """
    logging.info("Copying templates into '%s' helm chart directory ...", helmChart)

    if preservedFiles is None:
        preservedFiles = []

    # Determine directory path
    directoryPath = os.path.join(outputDir, "charts", "toggle", helmChart)

    # Remove existing files if directory exists
    if os.path.exists(directoryPath):
        logging.debug("Removing existing template files...")
        for filename in os.listdir(os.path.join(directoryPath, "templates")):
            if filename not in preservedFiles:
                filepath = os.path.join(directoryPath, "templates", filename)
                os.remove(filepath)
        logging.debug("Existing template files removed.")

    else:
        # Create directory and template subdirectory
        logging.debug("Creating new directory for the helm chart...")
        os.makedirs(os.path.join(directoryPath, "templates"))
        logging.debug("New directory created.")

    logging.debug("Copying template files...")
    for template_file in ["Chart.yaml", "values.yaml"]:
        shutil.copyfile(
            os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates", template_file),
            os.path.join(directoryPath, template_file)
        )
    logging.debug("Template files copied.")
    logging.info("Templates successfully copied into the '%s' helm chart directory.", helmChart)

# Fill in the chart.yaml template with information from the CSV
def fillChartYaml(helmChart, name, csvPath):
    """_summary_

    Args:
        helmChart (_type_): _description_
        name (_type_): _description_
        csvPath (_type_): _description_
    """
    logging.info("Updating '%s' Chart.yaml file ...", helmChart)
    chartYml = os.path.join(helmChart, "Chart.yaml")

    # Read Chart.yaml
    with open(chartYml, 'r', encoding='utf-8') as f:
        chart = yaml.safe_load(f)

    # logging.info("%s", csvPath)
    # Read CSV
    with open(csvPath, 'r', encoding='utf-8') as f:
        csv = yaml.safe_load(f)

    logging.info("Chart Name: %s", helmChart)


    # Write to Chart.yaml
    chart['name'] = name

    if 'metadata' in csv:
        if 'annotations' in csv ["metadata"]:
            if 'description' in csv["metadata"]["annotations"]:
                logging.info("Description: %s", csv["metadata"]["annotations"]["description"])
                chart['description'] = csv["metadata"]["annotations"]["description"]
    # chart['version'] = csv['metadata']['name'].split(".", 1)[1][1:]
    with open(chartYml, 'w', encoding='utf-8') as f:
        yaml.dump(chart, f)
    logging.info("'%s' Chart.yaml updated successfully.\n", helmChart)

# Copy chart-templates/deployment, update it with CSV deployment information, and add to chart
def add_deployment(helmChart, deployment):
    """_summary_

    Args:
        helmChart (_type_): _description_
        deployment (_type_): _description_
    """
    name = deployment["name"]
    logging.info("Templating deployment '%s.yaml' ...", name)

    deployYaml = os.path.join(helmChart, "templates",  name + ".yaml")
    shutil.copyfile(os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/deployment.yaml"), deployYaml)

    with open(deployYaml, 'r', encoding='utf-8') as f:
        deploy = yaml.safe_load(f)

    deploy['spec'] = deployment['spec']
    if 'spec' in deploy:
        if 'template' in deploy['spec']:
            if 'spec' in deploy['spec']['template']:
                if 'imagePullPolicy' in deploy['spec']['template']['spec']:
                    del deploy['spec']['template']['spec']['imagePullPolicy']
    deploy['metadata']['name'] = name
    with open(deployYaml, 'w', encoding='utf-8') as f:
        yaml.dump(deploy, f)
    logging.info("Deployment '%s.yaml' updated successfully.\n", name)

# Copy chart-templates/clusterrole,clusterrolebinding,serviceaccount.yaml update it with CSV information, and add to chart
def add_cluster_scoped_rbac(helmChart, rbacMap):
    """_summary_

    Args:
        helmChart (_type_): _description_
        rbacMap (_type_): _description_
    """
    name = rbacMap["serviceAccountName"]
    # name = "not-default"

    logging.info("Setting cluster scoped RBAC ...")
    logging.info("Templating clusterrole '%s-clusterrole.yaml' ...", name)

    # Create Clusterrole
    clusterroleYaml = os.path.join(helmChart, "templates",  name + "-clusterrole.yaml")
    shutil.copyfile(os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/clusterrole.yaml"), clusterroleYaml)
    with open(clusterroleYaml, 'r', encoding='utf-8') as f:
        clusterrole = yaml.safe_load(f)
    # Edit Clusterrole
    clusterrole["rules"] = rbacMap["rules"]
    clusterrole["metadata"]["name"] = name
    # Save Clusterrole
    with open(clusterroleYaml, 'w', encoding='utf-8') as f:
        yaml.dump(clusterrole, f)
    logging.info("Clusterrole '%s-clusterrole.yaml' updated successfully.", name)

    logging.info("Templating serviceaccount '%s-serviceaccount.yaml' ...", name)
    # Create Serviceaccount
    serviceAccountYaml = os.path.join(helmChart, "templates",  name + "-serviceaccount.yaml")
    shutil.copyfile(os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/serviceaccount.yaml"), serviceAccountYaml)
    with open(serviceAccountYaml, 'r', encoding='utf-8') as f:
        serviceAccount = yaml.safe_load(f)
    # Edit Serviceaccount
    serviceAccount["metadata"]["name"] = name
    # Save Serviceaccount
    with open(serviceAccountYaml, 'w', encoding='utf-8') as f:
        yaml.dump(serviceAccount, f)
    logging.info("Serviceaccount '%s-serviceaccount.yaml' updated successfully.", name)

    logging.info("Templating clusterrolebinding '%s-clusterrolebinding.yaml' ...", name)
    # Create Clusterrolebinding
    clusterrolebindingYaml = os.path.join(helmChart, "templates",  name + "-clusterrolebinding.yaml")
    shutil.copyfile(os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/clusterrolebinding.yaml"), clusterrolebindingYaml)
    with open(clusterrolebindingYaml, 'r', encoding='utf-8') as f:
        clusterrolebinding = yaml.safe_load(f)
    clusterrolebinding['metadata']['name'] = name
    clusterrolebinding['roleRef']['name'] = clusterrole["metadata"]["name"]
    clusterrolebinding['subjects'][0]['name'] = name
    with open(clusterrolebindingYaml, 'w', encoding='utf-8') as f:
        yaml.dump(clusterrolebinding, f)
    logging.info("Clusterrolebinding '%s-clusterrolebinding.yaml' updated successfully.", name)
    logging.info("Cluster scoped RBAC created.\n")

# Copy over role, rolebinding, and serviceaccount templates from chart-templates/templates, update with CSV information, and add to chart
def add_namespace_scoped_rbac(helmChart, rbacMap):
    """_summary_

    Args:
        helmChart (_type_): _description_
        rbacMap (_type_): _description_
    """
    name = rbacMap["serviceAccountName"]
    # name = "not-default"
    logging.info("Setting namespaced scoped RBAC ...")
    logging.info("Templating role '%s-role.yaml' ...", name)
    # Create role
    roleYaml = os.path.join(helmChart, "templates",  name + "-role.yaml")
    shutil.copyfile(os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/role.yaml"), roleYaml)
    with open(roleYaml, 'r', encoding='utf-8') as f:
        role = yaml.safe_load(f)
    # Edit role
    role["rules"] = rbacMap["rules"]
    role["metadata"]["name"] = name
    # Save role
    with open(roleYaml, 'w', encoding='utf-8') as f:
        yaml.dump(role, f)
    logging.info("Role '%s-role.yaml' updated successfully.", name)

    # Create Serviceaccount
    serviceAccountYaml = os.path.join(helmChart, "templates",  name + "-serviceaccount.yaml")
    if not os.path.isfile(serviceAccountYaml):
        logging.info("Serviceaccount doesnt exist. Templating '%s-serviceaccount.yaml' ...", name)
        shutil.copyfile(os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/serviceaccount.yaml"), serviceAccountYaml)
        with open(serviceAccountYaml, 'r', encoding='utf-8') as f:
            serviceAccount = yaml.safe_load(f)
        # Edit Serviceaccount
        serviceAccount["metadata"]["name"] = name
        # Save Serviceaccount
        with open(serviceAccountYaml, 'w', encoding='utf-8') as f:
            yaml.dump(serviceAccount, f)
        logging.info("Serviceaccount '%s-serviceaccount.yaml' updated successfully.", name)

    logging.info("Templating rolebinding '%s-rolebinding.yaml' ...", name)
    # Create rolebinding
    rolebindingYaml = os.path.join(helmChart, "templates",  name + "-rolebinding.yaml")
    shutil.copyfile(os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/rolebinding.yaml"), rolebindingYaml)
    with open(rolebindingYaml, 'r', encoding='utf-8') as f:
        rolebinding = yaml.safe_load(f)
    rolebinding['metadata']['name'] = name
    rolebinding['roleRef']['name'] = role["metadata"]["name"] = name
    rolebinding['subjects'][0]['name'] = name
    with open(rolebindingYaml, 'w', encoding='utf-8') as f:
        yaml.dump(rolebinding, f)
    logging.info("Rolebinding '%s-rolebinding.yaml' updated successfully.", name)
    logging.info("Namespace scoped RBAC created.\n")

def process_csv_section(csv_data, section, handler_func, helm_chart):
    """_summary_

    Args:
        csv_data (_type_): _description_
        section (_type_): _description_
        handler_func (_type_): _description_
        helm_chart (_type_): _description_
    """
    section_data = csv_data.get('spec', {}).get('install', {}).get('spec', {}).get(section)
    if section_data:
        for item in section_data:
            handler_func(helm_chart, item)

def check_unsupported_csv_resources(csv_path, csv_data, supported_config_types):
    """Check if there are unsupported resource types in the CSV."""
    unsupported_resources = [
        resource for resource in csv_data['spec']['install']['spec']
        if resource not in supported_config_types
    ]

    if unsupported_resources:
        logging.error("Found unsupported resources in the CSV: '%s' in '%s'",
                      ", ".join(unsupported_resources), csv_path)
        logging.error("Some resources in the CSV are not supported. Please review the CSV file.")
        return True

    return False

def escape_template_variables(helmChart, variables, branch):
    """_summary_

    Args:
        helmChart (_type_): _description_
        variables (_type_): _description_
        branch (_type_): _description_
    """
    addonTemplates = find_templates_of_type(helmChart, 'AddOnTemplate', branch)
    for addonTemplate in addonTemplates:
        for variable in variables:
            logging.info("Start to escape vriable %s", variable)
            at = open(addonTemplate, "r")
            lines = at.readlines()
            v = "{{"+variable+"}}"
            for i, line in enumerate(lines):
                if v in line.strip():
                    logging.info("Found variable %s in line: %s", v, line.strip())
                    lines[i] = line.replace(v, "{{ `"+ v + "` }}")

            a_file = open(addonTemplate, "w")
            a_file.writelines(lines)
            a_file.close()
    logging.info("Escaped template variables.\n")

# Adds resources identified in the CSV to the helmchart
def extract_csv_resources(helm_chart, csv_path):
    """_summary_

    Args:
        helm_chart (_type_): _description_
        csv_path (_type_): _description_
    """
    logging.info("Reading CSV file: '%s'", csv_path)

    try:
        with open(csv_path, 'r', encoding='utf-8') as f:
            csv_data = yaml.safe_load(f)
    except Exception as e:
        logging.error("Unexpected error occured while processing file '%s': %s", csv_path, e)
        return

    logging.info("Checking for deployments, clusterpermissions, and permissions.\n")
    supported_csv_install_spec_types = ["customResourceDefinitions","clusterPermissions", "deployments", "permissions"]

    # Process deployments
    process_csv_section(csv_data, "deployments", add_deployment, helm_chart)

    # Process clusterPermissions (ClusterRoles)
    process_csv_section(csv_data, "clusterPermissions", add_cluster_scoped_rbac, helm_chart)

    # Process permissions (Roles)
    process_csv_section(csv_data, "permissions", add_namespace_scoped_rbac, helm_chart)

    logging.info("Resources have been successfully added to chart '%s' from CSV '%s'.\n", helm_chart, csv_path)
    if check_unsupported_csv_resources(csv_data, csv_data, supported_csv_install_spec_types):
        sys.exit(1)

# Copies additional resources from the CSV directory to the Helm chart
def copy_additional_resources(helmChart, csvPath, branch):
    """_summary_

    Args:
        helmChart (_type_): _description_
        csvPath (_type_): _description_
        branch (_type_): _description_
    """
    logging.info("Copying additional resources from the bundle manifests if present ...")

    dirPath = os.path.dirname(csvPath)
    logging.info("Reading resources from directory: '%s'", dirPath)

    # List of resources that are required for the OLM bundle (currently, empty but can be expanded)
    required_bundle_resource_types = []

    # List of optional resources that are supported by the OLM bundle
    optional_supported_bundle_resourceTypes = ["ClusterRole", "ClusterRoleBinding", "ConfigMap", "ConsoleCLIDownload",
    "ConsoleLink", "ConsoleQuickStart", "ConsoleYamlSample", "PodDisruptionBudget", "PriorityClass", "PrometheusRule",
    "Role", "RoleBinding", "Secret", "Service", "ServiceAccount", "ServiceMonitor", "VerticalPodAutoscaler"]

    # List of resources that are allowed but not be explicitly handled by the OLM bundle
    allowed_bundle_resource_types = ["AddOnTemplate", "ClusterManagementAddOn"]

    # List of resources that should be ignored or excluded from the copy process (not copied to Helm chart)
    ignored_or_excluded_bundle_resource_types = ["ClusterServiceVersion", "CustomResourceDefinition"]

    # List of resources that should be **expected** in the OLM bundle, including both required and optional resources.
    expected_bundle_resource_types = required_bundle_resource_types + optional_supported_bundle_resourceTypes

    # List of collected unsupported resource types found in the bundle manifest
    unsupported_resources = []

    for filename in os.listdir(dirPath):
        if filename.endswith(".yaml") or filename.endswith(".yml"):
            filePath = os.path.join(dirPath, filename)

            # Try to load all documents to check if it's multi-document
            try:
                with open(filePath, 'r', encoding='utf-8') as f:
                    docs = list(yaml.safe_load_all(f))
            except Exception as e:
                logging.error("Unexpected error occured while processing file '%s': %s", filePath, e)
                continue

            # Check if this is a multi-document file AND version supports splitting
            is_multi_document = len(docs) > 1
            should_split = is_multi_document and is_version_compatible(branch, '2.16', '2.11', '2.16')

            # If not splitting (single document OR old version), just copy the file as-is
            if not should_split:
                # Use original single-document logic - just copy the file
                fileYml = docs[0] if docs else None
                if not fileYml:
                    continue

                resourceKind = fileYml.get("kind", None)
                if resourceKind is None:
                    logging.warning("Skipping file '%s' as it does not define a 'kind' attribute.", filename)
                    continue

                # Skip ignored or excluded resource types
                if resourceKind in ignored_or_excluded_bundle_resource_types:
                    logging.warning("Skipping ignored/excluded resource type '%s' from file '%s'.", resourceKind, filename)
                    continue

                # Handle white-listed resources
                elif resourceKind in allowed_bundle_resource_types:
                    logging.info("Copying white listed resource '%s' from file '%s' to Helm chart.", resourceKind, filename)
                    shutil.copyfile(filePath, os.path.join(helmChart, "templates", os.path.basename(filePath)))
                    continue

                # Handle expected resources
                elif resourceKind in expected_bundle_resource_types:
                    logging.info("Copying expected resource type '%s' from file '%s' to Helm chart.", resourceKind, filename)
                    shutil.copyfile(filePath, os.path.join(helmChart, "templates", os.path.basename(filePath)))
                    continue

                # Log unsupported resources
                else:
                    logging.warning("Unsupported resource type '%s' found in file '%s'.", resourceKind, filename)
                    unsupported_resources.append(resourceKind)

            else:
                # Multi-document file on 2.16+ - split and rename each document
                for doc in docs:
                    if not doc:
                        continue

                    # Extract the 'kind' of the resource from the YAML document
                    resourceKind = doc.get("kind", None)
                    if resourceKind is None:
                        logging.warning("Skipping document in file '%s' as it does not define a 'kind' attribute.", filename)
                        continue

                    # Skip ignored or excluded resource types
                    if resourceKind in ignored_or_excluded_bundle_resource_types:
                        logging.warning("Skipping ignored/excluded resource type '%s' from file '%s'.", resourceKind, filename)
                        continue

                    # Handle white-listed resources (allowed but not handled by the OLM bundle)
                    elif resourceKind in allowed_bundle_resource_types:
                        resource_name = doc.get('metadata', {}).get('name')
                        if not resource_name:
                            logging.warning("Skipping resource in file '%s' as it does not have a metadata.name field.", filename)
                            continue

                        # Generate filename based on resource name and kind (matching webhook pattern)
                        new_filename = f"{resource_name.lower()}-{resourceKind.lower()}.yaml"
                        dest_file_path = os.path.join(helmChart, "templates", new_filename)

                        logging.info("Copying white listed resource '%s/%s' from file '%s' to Helm chart as '%s'.",
                                     resourceKind, resource_name, filename, new_filename)

                        with open(dest_file_path, 'w', encoding='utf-8') as f:
                            yaml.dump(doc, f, width=float("inf"), default_flow_style=False, allow_unicode=True)
                        continue

                    # Handle expected resources (both required and optional)
                    elif resourceKind in expected_bundle_resource_types:
                        resource_name = doc.get('metadata', {}).get('name')
                        if not resource_name:
                            logging.warning("Skipping resource in file '%s' as it does not have a metadata.name field.", filename)
                            continue

                        # Generate filename based on resource name and kind (matching webhook pattern)
                        new_filename = f"{resource_name.lower()}-{resourceKind.lower()}.yaml"
                        dest_file_path = os.path.join(helmChart, "templates", new_filename)

                        logging.info("Copying expected resource type '%s/%s' from file '%s' to Helm chart as '%s'.",
                                     resourceKind, resource_name, filename, new_filename)

                        with open(dest_file_path, 'w', encoding='utf-8') as f:
                            yaml.dump(doc, f, width=float("inf"), default_flow_style=False, allow_unicode=True)
                        continue

                    # Log unsupported resources
                    else:
                        logging.warning("Unsupported resource type '%s' found in file '%s'.", resourceKind, filename)
                        unsupported_resources.append(resourceKind)

    if unsupported_resources:
        logging.error("Found unsupported resources in the bundle manifest: %s. Terminating process.",
            ", ".join(set(unsupported_resources)))  # Use `set` to avoid duplicates
        sys.exit(1)

def print_title(title: str):
    separator = '-' * (len(title) + 10)
    logging.info(separator)
    logging.info(f"{title.center(len(separator))}")
    logging.info(separator)

# Copies webhook resources from the target directory to the Helm chart
def copy_webhook_configuration_manifests(dest_helm_chart_path, webhook_path):
    """_summary_

    Args:
        dest_helm_chart_path (_type_): _description_
        webhook_path (_type_): _description_
    """
    logging.info("Copying webhook configuration resources from the repo if present ...")

    # Check if webhook_path itself is a directory
    if not os.path.exists(webhook_path) or not os.path.isfile(webhook_path):
        logging.warning("Webhook file not found: '%s'. Skipping webhook creation.", webhook_path)
        return

    logging.info("Found webhook configuration file: '%s'", webhook_path)
    with open(webhook_path, 'r') as file:
        content = file.read()

    # Split the content by '---' if it's a multi-document YAML file
    output = content.split('---')
    for _, doc in enumerate(output):
        try:
            # Load the YAML content of the document
            yaml_content = yaml.safe_load(doc)
            if yaml_content is None:
                logging.warning("Skipped empty or invalid YAML content during template processing")
                continue

            # Extract the kind and name from the resource
            kind = yaml_content.get('kind')
            name = yaml_content.get('metadata', {}).get('name')

            # if not kind or not name:
            logging.warning(
                f"YAML content is missing a kind or name attribute. Skipping resource processing: {yaml_content}")

            # Generate the new filename based on kind and name
            new_filename = f"{name.lower()}-{kind.lower()}.yaml"

            # Path to save the new file
            new_file_path = os.path.join(dest_helm_chart_path, "templates", new_filename)

            # Ensure the templates directory exists
            os.makedirs(os.path.dirname(new_file_path), exist_ok=True)

            # Write the YAML content to the new file
            with open(new_file_path, 'w') as new_file:
                yaml.dump(yaml_content, new_file, default_flow_style=False, sort_keys=True)

        except Exception as e:
            logging.warning(f"Unexpected error occurred while processing yaml content: {e}")

def ensure_webhook_namespace(resource_data, resource_name, default_namespace):
    """
    Ensures that the namespace for webhooks in a MutatingWebhookConfiguration or 
    ValidatingWebhookConfiguration is set correctly. Uses Helm templating to 
    apply a default namespace if none is provided.
    Args:
        resource_data (dict): The webhook configuration resource data.
        resource_name (str): The name of the webhook resource.
        default_namespace (str): The default namespace to use if not set.
    Returns:
        None: Modifies resource_data in place.
    """
    if 'webhooks' not in resource_data:
        return

    for webhook in resource_data.get('webhooks', []):
        client_config = webhook.get('clientConfig', {})

        service = client_config.get('service')
        if not service:
            continue

        service_name = service.get('name')
        service_namespace = service.get('namespace')
        service_path = service.get('path')

        if service_namespace is None:
            # Use the default Helm namespace if not specified
            service_namespace = default_namespace

        else:
            # Update Helm templating to override existing namespace
            service_namespace = f"{{{{ default \"{service_namespace}\" .Values.global.namespace }}}}"

        service['namespace'] = service_namespace

        # Log details for each distinct service
        logging.info(f"Webhook service for '{resource_name}' set to:")
        logging.info(f"  Name: {service_name}")
        logging.info(f"  Namespace: {service_namespace}")
        logging.info(f"  Path: {service_path}\n")

# updateHelmResources adds standard configuration to the generic kubernetes resources
def update_helm_resources(chartName, helmChart, skip_rbac_overrides, exclusions, inclusions, branch, preserved_files=[]):
    logging.info(f"Updating resources chart: {chartName}")

    resource_kinds = [
        "ClusterRole", "ClusterRoleBinding", "ConfigMap", "Deployment", "MutatingWebhookConfiguration",
        "NetworkPolicy", "PersistentVolumeClaim", "RoleBinding", "Role", "Route", "Secret", "Service", "StatefulSet",
        "ValidatingWebhookConfiguration", "Job", "ConsolePlugin"
    ]

    namespace_scoped_kinds = [
        "ConfigMap", "Deployment", "NetworkPolicy", "PersistentVolumeClaim", "RoleBinding", "Role", "Route",
        "Secret", "Service", "StatefulSet", "Job"
    ]

    for kind in resource_kinds:
        resource_templates = find_templates_of_type(helmChart, kind, branch)
        if not resource_templates:
            logging.info("------------------------------------------")
            logging.warning(f"No {kind} templates found in the Helm chart [Skipping]")
            logging.info("------------------------------------------\n")
            continue

        else:
            logging.info("------------------------------------------")
            logging.info(f"Found {len(resource_templates)} {kind} templates")
            logging.info("------------------------------------------")

        # Set the default namespace for the chart.
        default_namespace = """{{ .Values.global.namespace }}"""

        for template_path in resource_templates:
            if os.path.basename(template_path) in preserved_files:
                logging.warning("File '%s' is marked as preserved, skipping template update.", template_path)
                continue  # Skip this file

            try:
                with open(template_path, 'r') as f:
                    resource_data = yaml.safe_load(f)
                    resource_name = resource_data['metadata'].get('name')
                    logging.info(f"Processing resource: {resource_name} from template: {template_path}")

                # Ensure namespace is set for namespace-scoped resources
                if kind in namespace_scoped_kinds:
                    resource_namespace = resource_data['metadata'].get('namespace')

                    if resource_namespace is None or resource_namespace == "PLACEHOLDER_NAMESPACE":
                        # Use the default Helm namespace if not specified
                        resource_namespace = default_namespace
                    else:
                        # Allow Helm templating to override existing namespace
                        resource_namespace = f"{{{{ default \"{resource_namespace}\" .Values.global.namespace }}}}"

                    resource_data['metadata']['namespace'] = resource_namespace
                    logging.info(f"Namespace for '{resource_name}' set to: {resource_namespace}")

                # Ensure Mutating/Validating WebhookConfigurations has a service namespace set,
                # defaulting to Helm values if not specified.
                if kind == "MutatingWebhookConfiguration" or kind == "ValidatingWebhookConfiguration":
                    ensure_webhook_namespace(resource_data, resource_name, default_namespace)

                with open(template_path, 'w') as f:
                    yaml.dump(resource_data, f, width=float("inf"), default_flow_style=False, allow_unicode=True)
                    logging.info(f"Succesfully updated resource: {resource_name}\n")

            except Exception as e:
                logging.error(f"Error processing template '{template_path}': {e}")

    logging.info("Resource updating process completed.")

# Given a resource Kind, return all filepaths of that resource type in a chart directory
def find_templates_of_type(helmChart, kind, branch):
    """_summary_

    Args:
        helmChart (_type_): _description_
        kind (_type_): _description_
        branch (_type_): _description_

    Returns:
        _type_: _description_
    """
    resources = []
    for filename in os.listdir(os.path.join(helmChart, "templates")):
        if filename.endswith(".yaml") or filename.endswith(".yml"):
            filePath = os.path.join(helmChart, "templates", filename)
            with open(filePath, 'r', encoding='utf-8') as f:
                # Handle multi-document YAML files for ACM 2.16+, MCE 2.11+
                try:
                    if is_version_compatible(branch, '2.16', '2.11', '2.16'):
                        docs = list(yaml.safe_load_all(f))
                    else:
                        # Fallback to single-document for older versions
                        single_doc = yaml.safe_load(f)
                        docs = [single_doc] if single_doc else []
                    for doc in docs:
                        if doc and 'kind' in doc and doc['kind'] == kind:
                            resources.append(filePath)
                            break  # File matches, no need to check other docs
                except yaml.YAMLError:
                    continue
            continue
        else:
            continue
    return resources

# For each deployment, identify the image references if any exist in the environment variable fields, insert helm flow control code to reference it, and add image-key to the values.yaml file.
# If the image-key referenced in the deployment does not exist in `imageMappings` in the Config.yaml, this will fail. Images must be explicitly defined
def fixEnvVarImageReferences(helmChart, imageKeyMapping, branch, operator_name, apply=True):
    """_summary_

    Args:
        helmChart (_type_): _description_
        imageKeyMapping (_type_): _description_
        branch (_type_): _description_
        operator_name (_type_): _description_
        apply (bool): When False, validate mappings only without writing files.
    """
    logging.info("Fixing image references in container 'env' section in deployments and values.yaml ...")
    valuesYaml = os.path.join(helmChart, "values.yaml")
    deployments = find_templates_of_type(helmChart, 'Deployment', branch)

    errors = []
    pending_updates = []
    imageKeys = []
    for deployment in deployments:
        with open(deployment, 'r', encoding='utf-8') as f:
            deploy = yaml.safe_load(f)

        containers = deploy['spec']['template']['spec']['containers']
        for container in containers:
            if 'env' not in container:
                continue

            for env in container['env']:
                env_var_name = env['name']
                if not env_var_name.endswith('_IMAGE'):
                    continue
                repository = parse_image_ref(env['value'])['repository']
                image_key = lookup_image_key(repository, imageKeyMapping)
                if image_key is None:
                    errors.append(format_image_mapping_error(
                        operator_name,
                        os.path.relpath(deployment, helmChart),
                        f"env.{env_var_name}",
                        repository,
                    ))
                    logging.error(errors[-1])
                    continue
                imageKeys.append(image_key)
                if apply:
                    env['value'] = "{{ .Values.global.imageOverrides." + image_key + " }}"

        if apply:
            pending_updates.append((deployment, deploy))

    if errors or not apply:
        return errors

    for deployment, deploy in pending_updates:
        with open(deployment, 'w', encoding='utf-8') as f:
            yaml.dump(deploy, f)

    with open(valuesYaml, 'r', encoding='utf-8') as f:
        values = yaml.safe_load(f)
    for imageKey in imageKeys:
        values['global']['imageOverrides'][imageKey] = ""
    with open(valuesYaml, 'w', encoding='utf-8') as f:
        yaml.dump(values, f)
    logging.info("Image container env references in deployments and values.yaml updated successfully.\n")
    return errors

# For each deployment, identify the image references if any exist in the image field, insert helm flow control code to reference it, and add image-key to the values.yaml file.
# If the image-key referenced in the deployment does not exist in `imageMappings` in the Config.yaml, this will fail. Images must be explicitly defined
def fixImageReferences(helmChart, imageKeyMapping, branch, operator_name, apply=True):
    """_summary_

    Args:
        helmChart (_type_): _description_
        imageKeyMapping (_type_): _description_
        branch (_type_): _description_
        operator_name (_type_): _description_
        apply (bool): When False, validate mappings only without writing files.
    """
    logging.info("Fixing image and pull policy references in deployments and values.yaml ...")
    valuesYaml = os.path.join(helmChart, "values.yaml")
    deployments = find_templates_of_type(helmChart, 'Deployment', branch)
    errors = []
    pending_updates = []
    imageKeys = []
    for deployment in deployments:
        with open(deployment, 'r', encoding='utf-8') as f:
            deploy = yaml.safe_load(f)

        containers = deploy['spec']['template']['spec']['containers']
        for container in containers:
            repository = parse_image_ref(container['image'])["repository"]
            container_name = container.get('name', 'unknown')
            image_key = lookup_image_key(repository, imageKeyMapping)
            if image_key is None:
                errors.append(format_image_mapping_error(
                    operator_name,
                    os.path.relpath(deployment, helmChart),
                    f"container[{container_name}].image",
                    repository,
                ))
                logging.error(errors[-1])
                continue
            imageKeys.append(image_key)
            if apply:
                container['image'] = "{{ .Values.global.imageOverrides." + image_key + " }}"
                container['imagePullPolicy'] = "{{ .Values.global.pullPolicy }}"

        if apply:
            pending_updates.append((deployment, deploy))

    if errors or not apply:
        return errors

    for deployment, deploy in pending_updates:
        with open(deployment, 'w', encoding='utf-8') as f:
            yaml.dump(deploy, f)

    with open(valuesYaml, 'r', encoding='utf-8') as f:
        values = yaml.safe_load(f)
    try:
        del values['global']['imageOverrides']['imageOverride']
    except KeyError:
        pass
    for imageKey in imageKeys:
        values['global']['imageOverrides'][imageKey] = "" # set to temp to debug
    with open(valuesYaml, 'w', encoding='utf-8') as f:
        yaml.dump(values, f)
    logging.info("Image references and pull policy in deployments and values.yaml updated successfully.\n")
    return errors

# insert Heml flow control if/end block around a first and last line without changing
# the indexes of the lines list (so as to not mess up iteration across the lines).
def insertFlowControlIfAround(lines_list, first_line_index, last_line_index, if_condition):
    """_summary_

    Args:
        lines_list (_type_): _description_
        first_line_index (_type_): _description_
        last_line_index (_type_): _description_
        if_condition (_type_): _description_
    """
    lines_list[first_line_index] = "{{- if %s }}\n%s" % (if_condition, lines_list[first_line_index])
    lines_list[last_line_index] = "%s{{- end }}\n" % lines_list[last_line_index]

def is_version_compatible(branch, min_release_version, min_backplane_version, min_ocm_version, enforce_master_check=True):
    """_summary_

    Args:
        branch (_type_): _description_
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

        # Extract the version part from the branch name (e.g., '2.12-integration' -> '2.12')
        pattern = r'(\d+\.\d+)'  # Matches versions like '2.12'

        if branch == "main" or branch == "master":
            if enforce_master_check:
                return True
            else:
                return False

        match = re.search(pattern, branch)
        if match:
            v = match.group(1)  # Extract the version
            branch_version = version.Version(v)  # Create a Version object

            if "release-ocm" in branch:
                min_branch_version = version.Version(min_ocm_version)  # Use the minimum release version

            elif "release" in branch:
                min_branch_version = version.Version(min_release_version)  # Use the minimum release version

            elif "backplane" in branch or "mce" in branch:
                min_branch_version = version.Version(min_backplane_version)  # Use the minimum backplane version

            else:
                logging.error("Unrecognized branch type for branch: %s", branch)
                return False

            # Check if the branch version is compatible with the specified minimum branch
            return branch_version >= min_branch_version

        else:
            logging.error("Version not found in branch: %s", branch)
            return False
        
    if acm_release_version and version.Version(acm_release_version) >= version.Version(min_release_version):
        return True

    elif mce_release_version and version.Version(mce_release_version) >= version.Version(min_backplane_version):
        return True

    else:
        return False

# injectHelmFlowControl injects advanced helm flow control which would typically make a .yaml file more difficult to parse. This should be called last.
def injectHelmFlowControl(deployment, sizes, branch):
    """_summary_

    Args:
        deployment (_type_): _description_
        sizes (_type_): _description_
        branch (_type_): _description_

    Returns:
        _type_: _description_
    """
    logging.info("Adding Helm flow control for NodeSelector, Proxy Overrides and SecCompProfile...")
    deploy = open(deployment, "r")
    with open(deployment, 'r', encoding='utf-8') as f:
        deployx = yaml.safe_load(f)
    lines = deploy.readlines()
    for i, line in enumerate(lines):
        if line.strip() == "nodeSelector: \'\'":
            lines[i] = """{{- with .Values.hubconfig.nodeSelector }}
      nodeSelector:
{{ toYaml . | indent 8 }}
{{- end }}
"""
        if line.strip() == "imagePullSecrets: \'\'":
            lines[i] = """{{- if .Values.global.pullSecret }}
      imagePullSecrets:
      - name: {{ .Values.global.pullSecret }}
{{- end }}
"""
        if line.strip() == "tolerations: \'\'":
            lines[i] = """{{- with .Values.hubconfig.tolerations }}
      tolerations:
      {{- range . }}
      - {{ if .Key }} key: {{ .Key }} {{- end }}
        {{ if .Operator }} operator: {{ .Operator }} {{- end }}
        {{ if .Value }} value: {{ .Value }} {{- end }}
        {{ if .Effect }} effect: {{ .Effect }} {{- end }}
        {{ if .TolerationSeconds }} tolerationSeconds: {{ .TolerationSeconds }} {{- end }}
        {{- end }}
{{- end }}
"""

        if line.strip() == "env:" or line.strip() == "env: {}" or line.strip() == "env: []":
            lines[i] = """        env:
{{- if .Values.hubconfig.proxyConfigs }}
        - name: HTTP_PROXY
          value: {{ .Values.hubconfig.proxyConfigs.HTTP_PROXY }}
        - name: HTTPS_PROXY
          value: {{ .Values.hubconfig.proxyConfigs.HTTPS_PROXY }}
        - name: NO_PROXY
          value: {{ .Values.hubconfig.proxyConfigs.NO_PROXY }}
{{- end }}
"""

        if is_version_compatible(branch, '9.9', '9.9', '9.9', False):
            if 'replicas:' in line.strip():
                lines[i] = """  replicas: {{ .Values.hubconfig.replicaCount }}
"""

        if sizes:
            for sizDeployment in sizes["deployments"]:
                if sizDeployment["name"] == deployx["metadata"]["name"]:
                    for container in sizDeployment["containers"]:
                        if line.strip() == "resources: REPLACE-" + container["name"]:
                            lines[i] = """        resources:
{{-  if eq .values.hubconfig.hubSize "Small" }}
          limits:
            cpu: """ + container["Small"]["limits"]["cpu"] + """
            memory: """ + container["Small"]["limits"]["memory"] + """
          requests:
            cpu: """ + container["Small"]["requests"]["cpu"] + """
            memory: """ + container["Small"]["requests"]["memory"] + """
{{- end }}
{{ if eq .values.hubconfig.hubSize "Medium" }}
          limits:
            cpu: """ + container["Medium"]["limits"]["cpu"] + """
            memory: """ + container["Medium"]["limits"]["memory"] + """
          requests:
            cpu: """ + container["Medium"]["requests"]["cpu"] + """
            memory: """ + container["Medium"]["requests"]["memory"] + """
{{- end }}
{{-  if eq .values.hubconfig.hubSize "Large" }}
          limits:
            cpu: """ + container["Large"]["limits"]["cpu"] + """
            memory: """ + container["Large"]["limits"]["memory"] + """
          requests:
            cpu: """ + container["Large"]["requests"]["cpu"] + """
            memory: """ + container["Large"]["requests"]["memory"] + """
{{- end }}
{{ if eq .values.hubconfig.hubSize "ExtraLarge" }}
          limits:
            cpu: """ + container["ExtraLarge"]["limits"]["cpu"] + """
            memory: """ + container["ExtraLarge"]["limits"]["memory"] + """
          requests:
            cpu: """ + container["ExtraLarge"]["requests"]["cpu"] + """
            memory: """ + container["ExtraLarge"]["requests"]["memory"] + """
{{- end }}
"""
        if line.strip() == "seccompProfile:":
            next_line = lines[i+1]  # Ignore possible reach beyond end-of-list, not really possible
            if next_line.strip() == "type: RuntimeDefault":
                insertFlowControlIfAround(lines, i, i+1, "semverCompare \">=4.11.0\" .Values.hubconfig.ocpVersion")
                if is_version_compatible(branch, '9.9', '2.7', '2.12'):
                    insertFlowControlIfAround(lines, i, i+1, ".Values.global.deployOnOCP")
    #
    a_file = open(deployment, "w")
    a_file.writelines(lines)
    a_file.close()
    logging.info("Added Helm flow control for NodeSelector, Proxy Overrides and SecCompProfile.\n")

# inject_probe_config_helm_templates injects conditional probeConfig templates (ACM 2.17+)
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
    import re

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

# updateDeployments adds standard configuration to the deployments (antiaffinity, security policies, and tolerations)
def updateDeployments(helmChart, operator, exclusions, sizes, branch):
    """_summary_

    Args:
        helmChart (_type_): _description_
        operator (_type_): _description_
        exclusions (_type_): _description_
        sizes (_type_): _description_
        branch (_type_): _description_
    """
    logging.info("Updating deployments with antiaffinity, security policies, and tolerations ...")
    deploySpecYaml = os.path.join(os.path.dirname(os.path.realpath(__file__)), "chart-templates/templates/deploymentspec.yaml")
    with open(deploySpecYaml, 'r', encoding='utf-8') as f:
        deploySpec = yaml.safe_load(f)
    deployments = find_templates_of_type(helmChart, 'Deployment', branch)
    for deployment in deployments:
        with open(deployment, 'r', encoding='utf-8') as f:
            deploy = yaml.safe_load(f)
        affinityList = deploySpec['affinity']['podAntiAffinity']['preferredDuringSchedulingIgnoredDuringExecution']
        for antiaffinity in affinityList:
            antiaffinity['podAffinityTerm']['labelSelector']['matchExpressions'][0]['values'][0] = deploy['metadata']['name']

        pod_template = deploy['spec']['template']
        pod_template['metadata']['labels']['ocm-antiaffinity-selector'] = deploy['metadata']['name']
        if sizes:
            for  sizDeployment in sizes["deployments"]:
                if sizDeployment["name"] == deploy["metadata"]["name"]:
                    for i in deploy['spec']['template']['spec']['containers']:
                        if not any(d['name'] == i['name'] for d in sizDeployment["containers"]):
                            logging.error("Missing container in sizes.yaml")
                            sys.exit(1)
                        for sizContainer in sizDeployment["containers"]:
                            if sizContainer["name"] == i["name"]:
                                i['resources'] = 'REPLACE-' + i['name']

        pod_template_spec = pod_template['spec']
        pod_template_spec['affinity'] = deploySpec['affinity']
        pod_template_spec['tolerations'] = ''
        pod_template_spec['hostNetwork'] = False
        pod_template_spec['hostPID'] = False
        pod_template_spec['hostIPC'] = False

        # Set automountServiceAccountToken only if is configured for the operator.
        if 'automountServiceAccountToken' in operator:
            automountSAToken = operator.get('automountServiceAccountToken')
            if isinstance(automountSAToken, bool):
                pod_template_spec['automountServiceAccountToken'] = operator.get('automountServiceAccountToken')
            else:
                logging.warning("automountServiceAccountToken should be a boolean. Ignoring invalid value.")

        pod_template_spec['nodeSelector'] = ""
        pod_template_spec['imagePullSecrets'] = ''

        with open(deployment, 'w', encoding='utf-8') as f:
            yaml.dump(deploy, f, width=float("inf"))

        logging.info("Deployments updated with antiaffinity, security policies, and tolerations successfully. \n")

        injectHelmFlowControl(deployment, sizes, branch)

        # Inject conditional probe config templates (ACM 2.17+)
        # This must be done AFTER injectHelmFlowControl since that function manipulates the YAML
        if is_version_compatible(branch, '2.17', '2.17', '2.17'):
            inject_probe_config_helm_templates(deployment)

# updateRBAC adds standard configuration to the RBAC resources (clusterroles, roles, clusterrolebindings, and rolebindings)
def updateRBAC(helmChart, branch):
    """_summary_

    Args:
        helmChart (_type_): _description_
        branch (_type_): _description_
    """
    logging.info("Updating clusterroles, roles, clusterrolebindings, and rolebindings ...")
    clusterroles = find_templates_of_type(helmChart, 'ClusterRole', branch)
    roles = find_templates_of_type(helmChart, 'Role', branch)
    clusterrolebindings = find_templates_of_type(helmChart, 'ClusterRoleBinding', branch)
    rolebindings = find_templates_of_type(helmChart, 'RoleBinding', branch)

    for rbacFile in clusterroles + roles + clusterrolebindings + rolebindings:
        with open(rbacFile, 'r', encoding='utf-8') as f:
            rbac = yaml.safe_load(f)
        rbac['metadata']['name'] = "{{ .Values.org }}:{{ .Chart.Name }}:" + rbac['metadata']['name']
        if rbac['kind'] in ['RoleBinding', 'ClusterRoleBinding']:
            rbac['roleRef']['name'] = "{{ .Values.org }}:{{ .Chart.Name }}:" + rbac['roleRef']['name']
        with open(rbacFile, 'w', encoding='utf-8') as f:
            yaml.dump(rbac, f)
    logging.info("Clusterroles, roles, clusterrolebindings, and rolebindings updated. \n")

def inject_security_context_constraints(resource, constraints_override):
    pod_template_spec = resource.setdefault("spec", {}).setdefault("template", {}).setdefault("spec", {})
    pod_security_context = pod_template_spec.setdefault("securityContext", {})

    # --- Pod-level security context ---
    pod_security_context['runAsNonRoot'] = constraints_override.get('runAsNonRoot', True)
    pod_security_context['runAsUser'] = constraints_override.get('runAsUser')
    pod_security_context['runAsGroup'] = constraints_override.get('runAsGroup')
    pod_security_context['fsGroup'] = constraints_override.get('fsGroup')
    pod_security_context['fsGroupChangePolicy'] = constraints_override.get('fsGroupChangePolicy')
    pod_security_context['SELinux'] = constraints_override.get('SELinux')
    pod_security_context['supplementalGroups'] = constraints_override.get('supplementalGroups')
    pod_security_context['supplementalGroupsPolicy'] = constraints_override.get('supplementalGroupsPolicy')

    pod_security_context.setdefault('seccompProfile', constraints_override.get('seccompProfile', {'type': 'RuntimeDefault'}))
    if pod_security_context.get('seccompProfile').get('type') != 'RuntimeDefault':
        logging.warning("Leaving non-standard pod-level seccompprofile setting.")

    # Remove keys with value None. We will only add those overrides, if there are custom values for us to pick up.
    for key in list(pod_security_context):
        if pod_security_context[key] is None:
            pod_security_context.pop(key)

    logging.info(f"Pod security context: {pod_security_context}")

    # --- Container-level security context ---
    container_constraints_override = constraints_override.get('containers', [])
    container_template_spec = pod_template_spec.get('containers', [])

    for container in container_template_spec:
        container_name = container.get('name')
        container_security_context = container.setdefault('securityContext', {})

        # Set container env to empty array, if it doesn't exist.
        container.setdefault('env', [])

        # Find matching constraint by container name
        matching_constraint = next((c for c in container_constraints_override if c.get('name') == container_name), {})

        container_security_context['allowPrivilegeEscalation'] = matching_constraint.get('allowPrivilegeEscalation', False)
        container_security_context['capabilities'] = matching_constraint.get('capabilities', {'drop': ['ALL']})
        container_security_context['privileged'] = matching_constraint.get('privileged', False)
        container_security_context['runAsNonRoot'] = matching_constraint.get('runAsNonRoot', True)
        container_security_context['readOnlyRootFilesystem'] = matching_constraint.get('readOnlyRootFilesystem', True)
        
        if 'seccompProfile'  in container_security_context:
            if container_security_context.get('seccompProfile').get('type') == 'RuntimeDefault':
                # Remove, to allow pod-level setting to have effect.
                del container_security_context['seccompProfile']
            else:
                logging.warning("Leaving non-standard pod-level seccompprofile setting.")

        logging.info(f"Container '{container_name}' security context: {container_security_context}\n")
        

def update_security_contexts(template_chart_path, constraints_override, branch):
    """_summary_

    Args:
        template_chart_path (_type_): _description_
        constraints (list, optional): _description_. Defaults to [].
        branch (_type_): _description_
    """
    log_header("Injecting security context constraints...")

    for kind in ["Deployment", "Job", "StatefulSet"]:
        resource_templates = find_templates_of_type(template_chart_path, kind, branch)
        if not resource_templates:
            continue
    
        for template in resource_templates:
            try:
                with open(template, 'r', encoding="utf-8") as f:
                    resource = yaml.safe_load(f)

                name = resource.get("metadata", {}).get("name")
                constraints = next(
                    (c for c in constraints_override if c.get("kind") == kind and c.get("name") == name), {}
                )

                if constraints:
                    logging.info("Injecting *custom* security context into '%s/%s'", kind, name)
                else:
                    logging.info("Injecting *default* security context into '%s/%s'", kind, name)

                inject_security_context_constraints(resource, constraints)
                with open(template, 'w', encoding="utf-8") as f:
                    yaml.dump(resource, f, width=float("inf"))

            except Exception as e:
                logging.error(f"Error injecting security context for {template}: {e}")

    logging.info("Updated security context for '%s'\n", template_chart_path)

def injectRequirements(helm_chart_path, operator, sizes, branch):
    """_summary_

    Args:
        helmChart (_type_): _description_
        operator (_type_): _description_
        exclusions (_type_): _description_
        sizes (_type_): _description_
        branch (_type_): _description_
    """
    logging.info("Updating Helm chart '%s' with onboarding requirements ...", helm_chart_path)
    image_key_mapping = operator.get("imageMappings", {})
    operator_name = operator.get("name") or os.path.basename(helm_chart_path)
    exclusions = operator.get("exclusions")
    inclusions = operator.get("inclusions")
    security_context_constraints = operator.get("security-context-constraints", [])
    skip_rbac_overrides = operator.get("skipRBACOverrides", True)
    preserved_files = operator.get("preserve_files", [])

    errors = []

    # Validate image mappings before making any chart updates.
    errors.extend(fixImageReferences(helm_chart_path, image_key_mapping, branch, operator_name, apply=False))
    errors.extend(fixEnvVarImageReferences(helm_chart_path, image_key_mapping, branch, operator_name, apply=False))
    errors.extend(fixImageReferencesForAddonTemplate(helm_chart_path, image_key_mapping, branch, operator_name, apply=False))
    if errors:
        logging.warning(
            "Skipping chart updates for '%s' due to %d image mapping error(s)",
            helm_chart_path,
            len(errors),
        )
        return errors

    fixImageReferences(helm_chart_path, image_key_mapping, branch, operator_name, apply=True)
    fixEnvVarImageReferences(helm_chart_path, image_key_mapping, branch, operator_name, apply=True)
    fixImageReferencesForAddonTemplate(helm_chart_path, image_key_mapping, branch, operator_name, apply=True)
    injectAnnotationsForAddonTemplate(helm_chart_path, branch)

    if is_version_compatible(branch, '2.10', '2.5', '2.10'):
        update_security_contexts(helm_chart_path, security_context_constraints, branch)

    if is_version_compatible(branch, '2.13', '2.7', '2.13'):
        update_helm_resources(operator_name, helm_chart_path, skip_rbac_overrides, exclusions, inclusions, branch, preserved_files)

    # Updates RBAC and deployment configuration in the Helm chart.
    updateRBAC(helm_chart_path, branch)
    updateDeployments(helm_chart_path, operator, exclusions, sizes, branch)

    logging.info("Updated Chart '%s' successfully\n", helm_chart_path)
    return []

def addCRDs(repo, operator, outputDir, branch, preservedFiles=None, overwrite=False):
    """
    Add Custom Resource Definitions (CRDs) to the specified output directory.

    Args:
        repo (str): The name of the repository.
        operator (dict): The configuration of the operator.
        outputDir (str): The directory where CRDs will be added.
        branch (str): The branch being processed for version compatibility checks.
        preservedFiles (list, optional): List of files to preserve. Defaults to None.
        overwrite (bool, optional): Whether to overwrite existing files. Defaults to False.

    Raises:
        ValueError: If bundlePath is not found or if CRD file copying fails.
    """
    logging.info("Adding Custom Resource Definitions (CRDs) for operator: %s", operator['name'])

    if 'bundlePath' in operator:
        manifestsPath = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp", repo, operator["bundlePath"])
        if not os.path.exists(manifestsPath):
            raise ValueError("Could not validate bundlePath at given path: " + operator["bundlePath"])
        else:
            logging.info("Using specified bundlePath for CRDs: %s", operator["bundlePath"])

    else:
        bundlePath = getBundleManifestsPath(repo, operator)
        manifestsPath = os.path.join(bundlePath, "manifests")
        logging.info("Using bundlePath derived from repository for CRDs: %s", bundlePath)

    if preservedFiles is None:
        preservedFiles = []

    directoryPath = os.path.join(outputDir, "crds", operator['name'])
    if os.path.exists(directoryPath):
        logging.debug("Removing existing CRD files...")
        for filename in os.listdir(directoryPath):
            if filename not in preservedFiles:
                filepath = os.path.join(directoryPath, filename)
                os.remove(filepath)
        logging.debug("Existing CRD files removed.")

    else:
        os.makedirs(directoryPath)
        logging.debug("Created directory for CRDs: %s", directoryPath)

    for filename in os.listdir(manifestsPath):
        if not filename.endswith(".yaml"):
            continue

        filepath = os.path.join(manifestsPath, filename)
        with open(filepath, 'r', encoding='utf-8') as f:
            # Handle multi-document YAML files for ACM 2.16+, MCE 2.11+
            if is_version_compatible(branch, '2.16', '2.11', '2.16'):
                docs = list(yaml.safe_load_all(f))
            else:
                # Fallback to single-document for older versions
                single_doc = yaml.safe_load(f)
                docs = [single_doc] if single_doc else []

        # Check each document for CRDs
        for doc in docs:
            if not doc or "kind" not in doc:
                continue

            if doc["kind"] == "CustomResourceDefinition":
                dest_file_path = os.path.join(outputDir, "crds", operator['name'], filename)
                if overwrite or not os.path.exists(dest_file_path):
                    shutil.copyfile(filepath, dest_file_path)
                    logging.info("CRD file copied: %s", filename)
                break  # Only copy the file once even if it has multiple CRDs

    logging.info("CRDs added successfully for operator: %s", operator['name'])

def getBundleManifestsPath(repo, operator):
    """
    getBundleManifestsPath returns the path to the manifests directory
    of the latest operator bundle available in the desired channel
    """
    if 'bundlePath' in operator:
        bundlePath = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp", repo, operator["bundlePath"])
        if not os.path.exists(bundlePath):
            logging.critical("Could not validate bundlePath at given path: %s", operator["bundlePath"])
            sys.exit(1)
        return bundlePath

    # check every bundle's metadata for its supported channels
    bundles_directory = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp", repo, operator["bundles-directory"])
    if not os.path.exists(bundles_directory):
        logging.critical("Could not find bundles at given path: %s", operator["bundles-directory"])
        sys.exit(1)

    latest_bundle_version = "0.0.0"
    directories = [dir for dir in os.listdir(bundles_directory) if os.path.isdir(os.path.join(bundles_directory, dir))]
    for dir_name in directories:
        bundle_path = os.path.join(bundles_directory, dir_name)

        # Read metadata annotations
        annotations_file = os.path.join(bundle_path, "metadata", "annotations.yaml")
        if not os.path.isfile(annotations_file):
            logging.critical("Could not find annotations at given path: %s", annotations_file)
            sys.exit(1)
        with open(annotations_file, 'r', encoding='utf-8') as f:
            annotations = yaml.safe_load(f)
            channels = annotations.get('annotations', {}).get('operators.operatorframework.io.bundle.channels.v1').split(',')
            if not channels:
                logging.critical("Could not find channels in annotations file at given path: %s", annotations_file)
                sys.exit(1)
            if operator["channel"] in channels:
                # compare semantic version based on directory name
                if version.parse(dir_name) > version.parse(latest_bundle_version):
                    latest_bundle_version = dir_name

    latest_bundle_path = os.path.join(bundles_directory, latest_bundle_version)
    return latest_bundle_path

def get_csv_path(repo, operator, branch):
    """_summary_

    Args:
        repo (_type_): _description_
        operator (_type_): _description_
        branch (_type_): _description_

    Returns:
        _type_: _description_
    """
    if 'bundlePath' in operator:
        manifests_path = os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp", repo, operator["bundlePath"])
        if not os.path.exists(manifests_path):
            logging.critical("Could not validate bundlePath at given path: %s", operator["bundlePath"])
            sys.exit(1)
        else:
            logging.info("Using specified bundlePath: %s", operator["bundlePath"])

    else:
        bundle_path = getBundleManifestsPath(repo, operator)
        manifests_path = os.path.join(bundle_path, "manifests")
        logging.info("Using bundlePath derived from repository: %s", bundle_path)

    logging.info("Searching for CSV file in directory: %s", manifests_path)
    for file_name in os.listdir(manifests_path):
        if not file_name.endswith(".yaml"):
            continue

        file_path = os.path.join(manifests_path, file_name)
        with open(file_path, 'r', encoding='utf-8') as f:
            # Handle multi-document YAML files for ACM 2.16+, MCE 2.11+
            if is_version_compatible(branch, '2.16', '2.11', '2.16'):
                docs = list(yaml.safe_load_all(f))
            else:
                # Fallback to single-document for older versions
                single_doc = yaml.safe_load(f)
                docs = [single_doc] if single_doc else []

        # Check if any document in the file is a ClusterServiceVersion
        for doc in docs:
            if doc and doc.get("kind") == "ClusterServiceVersion":
                logging.info("CSV file found: %s", file_path)
                return file_path

    logging.warning("No CSV file found in directory: %s", manifests_path)
    return None

# injectAnnotationsForAddonTemplate injects following annotations for deployments in the AddonTemplate:
# - target.workload.openshift.io/management: '{"effect": "PreferredDuringScheduling"}'
def injectAnnotationsForAddonTemplate(helmChart, branch):
    """_summary_

    Args:
        helmChart (_type_): _description_
        branch (_type_): _description_
    """
    logging.info("Injecting Annotations for deployments in the AddonTemplate ...")

    addonTemplates = find_templates_of_type(helmChart, 'AddOnTemplate', branch)
    for addonTemplate in addonTemplates:
        with open(addonTemplate, 'r', encoding='utf-8') as f:
            # Handle multi-document YAML files for ACM 2.16+, MCE 2.11+
            if is_version_compatible(branch, '2.16', '2.11', '2.16'):
                docs = list(yaml.safe_load_all(f))
            else:
                # Fallback to single-document for older versions
                single_doc = yaml.safe_load(f)
                docs = [single_doc] if single_doc else []

        modified = False
        for templateContent in docs:
            if not templateContent or templateContent.get('kind') != 'AddOnTemplate':
                continue

            agentSpec = templateContent.get('spec', {}).get('agentSpec', {})
            if 'workload' not in agentSpec:
                continue
            workload = agentSpec['workload']
            if 'manifests' not in workload:
                continue
            manifests = workload['manifests']
            for manifest in manifests:
                if manifest['kind'] == 'Deployment':
                    metadata = manifest['spec']['template']['metadata']
                    if 'annotations' not in metadata:
                        metadata['annotations'] = {}
                    if 'target.workload.openshift.io/management' not in metadata['annotations']:
                        metadata['annotations']['target.workload.openshift.io/management'] = '{"effect": "PreferredDuringScheduling"}'
                        modified = True

        if modified:
            with open(addonTemplate, 'w', encoding='utf-8') as f:
                yaml.dump_all(docs, f, width=float("inf"))
                logging.info("Annotations injected successfully. \n")

# fixImageReferencesForAddonTemplate identify the image references for every deployment in addontemplates, if any exist
# in the image field, insert helm flow control code to reference it, and add image-key to the values.yaml file.
# If the image-key referenced in the addon template deployment does not exist in `imageMappings` in the Config.yaml,
# this will fail. Images must be explicitly defined
def fixImageReferencesForAddonTemplate(helmChart, imageKeyMapping, branch, operator_name, apply=True):
    """_summary_

    Args:
        helmChart (_type_): _description_
        imageKeyMapping (_type_): _description_
        branch (_type_): _description_
        operator_name (_type_): _description_
        apply (bool): When False, validate mappings only without writing files.
    """
    logging.info("Fixing image references in addon templates and values.yaml ...")

    addonTemplates = find_templates_of_type(helmChart, 'AddOnTemplate', branch)
    errors = []
    all_imageKeys = []
    pending_updates = []

    for addonTemplate in addonTemplates:
        with open(addonTemplate, 'r', encoding='utf-8') as f:
            # Handle multi-document YAML files for ACM 2.16+, MCE 2.11+
            if is_version_compatible(branch, '2.16', '2.11', '2.16'):
                docs = list(yaml.safe_load_all(f))
            else:
                # Fallback to single-document for older versions
                single_doc = yaml.safe_load(f)
                docs = [single_doc] if single_doc else []

        for templateContent in docs:
            if not templateContent or templateContent.get('kind') != 'AddOnTemplate':
                continue

            agentSpec = templateContent.get('spec', {}).get('agentSpec', {})
            if 'workload' not in agentSpec:
                continue
            workload = agentSpec['workload']
            if 'manifests' not in workload:
                continue
            manifests = workload['manifests']

            for manifest in manifests:
                if manifest['kind'] in ['Deployment', 'Job']:
                    manifest_kind = manifest['kind']
                    manifest_name = manifest.get('metadata', {}).get('name', 'unknown')
                    containers = manifest['spec']['template']['spec']['containers']
                    for container in containers:
                        repository = parse_image_ref(container['image'])["repository"]
                        container_name = container.get('name', 'unknown')
                        image_key = lookup_image_key(repository, imageKeyMapping)
                        if image_key is None:
                            errors.append(format_image_mapping_error(
                                operator_name,
                                os.path.relpath(addonTemplate, helmChart),
                                f"{manifest_kind}[{manifest_name}].container[{container_name}].image",
                                repository,
                            ))
                            logging.error(errors[-1])
                            continue
                        all_imageKeys.append(image_key)
                        if apply:
                            container['image'] = "{{ .Values.global.imageOverrides." + image_key + " }}"
                            # container['imagePullPolicy'] = "{{ .Values.global.pullPolicy }}"

        if apply:
            pending_updates.append((addonTemplate, docs))

    if errors or not apply:
        return errors

    if not all_imageKeys:
        return errors

    for addonTemplate, docs in pending_updates:
        with open(addonTemplate, 'w', encoding='utf-8') as f:
            yaml.dump_all(docs, f, width=float("inf"))
        logging.info("AddOnTemplate updated with image override successfully. \n")

    valuesYaml = os.path.join(helmChart, "values.yaml")
    with open(valuesYaml, 'r', encoding='utf-8') as f:
        values = yaml.safe_load(f)
    if 'imageOverride' in values['global']['imageOverrides']:
        del values['global']['imageOverrides']['imageOverride']
    for imageKey in all_imageKeys:
        values['global']['imageOverrides'][imageKey] = "" # set to temp to debug
    with open(valuesYaml, 'w', encoding='utf-8') as f:
        yaml.dump(values, f, width=float("inf"))
    logging.info("Image references and pull policy in addon templates and values.yaml updated successfully.\n")
    return errors

def backup_operator_output(output_dir, operator_name):
    """Save existing destination files for an operator before updating."""
    backup_root = tempfile.mkdtemp(prefix="operator-backup-")
    backup = {"backup_root": backup_root, "chart_existed": False, "crds_existed": False}

    chart_path = os.path.join(output_dir, "charts", "toggle", operator_name)
    if os.path.exists(chart_path):
        chart_backup = os.path.join(backup_root, "chart")
        shutil.copytree(chart_path, chart_backup)
        backup["chart"] = chart_backup
        backup["chart_existed"] = True

    crds_path = os.path.join(output_dir, "crds", operator_name)
    if os.path.exists(crds_path):
        crds_backup = os.path.join(backup_root, "crds")
        shutil.copytree(crds_path, crds_backup)
        backup["crds"] = crds_backup
        backup["crds_existed"] = True

    return backup

def restore_operator_output(output_dir, operator_name, backup):
    """Restore destination files saved before this run."""
    chart_path = os.path.join(output_dir, "charts", "toggle", operator_name)
    if backup["chart_existed"]:
        if os.path.exists(chart_path):
            shutil.rmtree(chart_path)
        shutil.copytree(backup["chart"], chart_path)
    elif os.path.exists(chart_path):
        shutil.rmtree(chart_path)

    crds_path = os.path.join(output_dir, "crds", operator_name)
    if backup["crds_existed"]:
        if os.path.exists(crds_path):
            shutil.rmtree(crds_path)
        shutil.copytree(backup["crds"], crds_path)
    elif os.path.exists(crds_path):
        shutil.rmtree(crds_path)

    logging.warning(
        "Restored destination output for operator '%s' to its original state due to errors.",
        operator_name,
    )

def cleanup_operator_backup(backup):
    """Remove the temporary backup directory."""
    shutil.rmtree(backup["backup_root"], ignore_errors=True)

def main():
    """_summary_
    """
    logging.basicConfig(level=logging.INFO)
    logging.info("Script started.")

    ## Initialize ArgParser
    parser = argparse.ArgumentParser()
    parser.add_argument("--component", dest="component", type=str, required=False, help="If provided, only this component will be processed")
    parser.add_argument("--config", dest="config", type=str, required=False, help="If provided, this config file will be processed")
    parser.add_argument("--destination", dest="destination", type=str, required=False, help="Destination directory of the created helm chart")
    parser.add_argument("--skipOverrides", dest="skipOverrides", type=bool, help="If true, overrides such as helm flow control will not be applied")
    parser.add_argument("--lint", dest="lint", action='store_true', help="If true, bundles will only be linted to ensure they can be transformed successfully. Default is False.")
    parser.add_argument("--component-branch", dest="component_branch", action='append', type=str, required=False, help="Override branch for a specific component. Format: component_name:branch_name. Can be specified multiple times.")
    parser.add_argument("--component-fork", dest="component_fork", action='append', type=str, required=False, help="Override git repository URL for a specific component. Format: component_name:git_url. Can be specified multiple times.")
    parser.set_defaults(skipOverrides=False)
    parser.set_defaults(lint=False)

    args = parser.parse_args()
    component = args.component
    config_override = args.config
    destination = args.destination
    skipOverrides = args.skipOverrides
    lint = args.lint

    # Parse component branch overrides into a dictionary
    component_branch_overrides = {}
    if args.component_branch:
        for override in args.component_branch:
            if ':' not in override:
                logging.warning(f"Invalid component-branch format: '{override}'. Expected 'component_name:branch_name'. Skipping.")
                continue
            comp_name, branch_name = override.split(':', 1)
            component_branch_overrides[comp_name] = branch_name
            logging.info(f"Branch override configured: {comp_name} -> {branch_name}")

    # Parse component fork overrides into a dictionary
    component_fork_overrides = {}
    if args.component_fork:
        for override in args.component_fork:
            if ':' not in override:
                logging.warning(f"Invalid component-fork format: '{override}'. Expected 'component_name:git_url'. Skipping.")
                continue
            comp_name, git_url = override.split(':', 1)
            component_fork_overrides[comp_name] = git_url
            logging.info(f"Fork override configured: {comp_name} -> {git_url}")

    if lint == False and not destination:
        logging.critical("Destination directory is required when not linting.")
        sys.exit(1)

    # Load configuration file
    # config.yaml holds the configurations for Operator bundle locations to be used
    if config_override:
        root_override_path = os.path.join(ROOT_DIR, config_override)
        script_override_path = os.path.join(SCRIPT_DIR, config_override)

        if os.path.exists(root_override_path):
            config_yaml = root_override_path
        else:
            config_yaml = script_override_path
    else:
        config_yaml = os.path.join(SCRIPT_DIR, "config.yaml")

    if not os.path.exists(config_yaml):
        logging.critical("Configuration file '%s' not found. Exiting.", config_yaml)
        sys.exit(1)

    try:
        with open(config_yaml, 'r', encoding='utf-8') as f:
            config = yaml.safe_load(f)
        logging.info("Loaded configuration from '%s'", config_yaml)

    except Exception as e:
        logging.critical("Unexpected error while loading configuration '%s'", config_yaml)
        sys.exit(1)

    # Normalize config into a list of components
    if isinstance(config, dict):
        # Set global environment variables
        os.environ['ACM_RELEASE_VERSION'] = config.get('acm-release-version', '')
        os.environ['MCE_RELEASE_VERSION'] = config.get('mce-release-version', '')
        components = config.get("components", [])

    else:
        components = config

    # Optionally filter by a specific component
    if component:
        components = [repo for repo in components if repo.get("repo_name") == component]

    errors = []

    # Loop through each repo in the config.yaml
    for repo in components:
        # We support two ways of getting bundle input:
        #
        # - Picking up already generated input from a Github repo
        #
        #   Entries for this approach include a "github_ref" property specifying the
        #   Git repo we clone.  Such a repo can supply input for multiple operators
        #   (eg: community-poerators) so the per-operator properties are configured
        #   via the "operators" list.
        #
        # - Generating the input using a budnle-gen tool.
        #
        #   Entries for this approach include a "gen_command" property specifying
        #   the command to run.  Since we expect that bundle-gen tool is going to gen
        #   the input for only a single operator, the per-operator properties are
        #   structured as singletons rather than being in a list.
        #
        #   We assume the bundle-gen tool knows which repos and such it needs to use
        #   to do its job, but needs to be told a branch-name or Git SHA to use
        #   to obtain bundle input info.

        csvPath = ""

        # Retrieve the repository name from the configuration
        # Ensure 'repo_name' is present in the configuration to proceed with
        # the tool-specific tasks.
        repo_name = repo.get("repo_name")
        if not repo_name:
            logging.critical(
                "Missing required field 'repo_name'. "
                "This is necessary for generating tool-specific bundles. "
                "Please provide a valid repository name."
            )
            sys.exit(1)

        # Retrieve the branch name from the repository configuration
        # If the 'branch' is not defined, handle it appropriately.
        branch = repo.get("branch")
        if not branch:
            logging.critical(
                "Missing required field 'branch'. "
                "This is necessary for generating tool-specific bundles. "
                "Please provide a valid branch value."   
            )
            sys.exit(1)

        # Retrieve the GitHub reference (e.g., branch, tag, or pull request)
        # The 'github_ref' identifies the exact Git reference that triggered the workflow.
        github_ref = repo.get("github_ref")
        if github_ref:
            # Check for fork override first, then use config github_ref
            if repo_name in component_fork_overrides:
                git_url = component_fork_overrides[repo_name]
                logging.info(f"Using fork override for {repo_name}: {git_url}")
            else:
                git_url = github_ref

            logging.info("Cloning repository: %s from %s", repo_name, git_url)
            repo_path = os.path.join(SCRIPT_DIR, "tmp", repo_name)

            if os.path.exists(repo_path):
                shutil.rmtree(repo_path)

            repository = Repo.clone_from(git_url, repo_path)

            # Check for branch override first, then use config branch
            if repo_name in component_branch_overrides:
                branch_to_use = component_branch_overrides[repo_name]
                logging.info(f"Using branch override for {repo_name}: {branch_to_use}")
            else:
                branch_to_use = branch

            repository.git.checkout(branch_to_use)

            sizesyaml = repo_path + "/bundle/manifests/sizes.yaml"
            if os.path.isfile(sizesyaml):
                with open(sizesyaml, 'r', encoding='utf-8') as f:
                    sizes = yaml.safe_load(f)
            else:
                sizes = {}

        elif "gen_command" in repo:
            try:
                # repo.branch specifies the branch or SHA the tool should use for input.
                # repo.bundlePath specifies the directory into which the bundle manifest
                # should be generated, and where they are fetched from for chartifying.

                bundlePath = repo["bundlePath"]

                # Check if we should fetch SHA dynamically from git remote
                if "git_remote" in repo:
                    sha = fetch_sha_from_git_remote(repo["git_remote"], branch)
                else:
                    sha = repo["sha"]

            except KeyError:
                logging.critical("branch and bundlePath are required for tool-generated bundles")
                sys.exit(1)
            cmd = "%s %s %s %s" % (repo["gen_command"], branch, sha, bundlePath)

            logging.info("Running bundle-gen tool: %s", cmd)
            rc = os.system(cmd)
            if rc != 0:
                logging.critical("Bundle-generation script exited with errors.")
                sys.exit(1)

            # Convert the repo entry  to the format used for Github-sourced bundles
            # so we can use a common path for both below.
            op = {
               "name": repo.get("name"),
               "imageMappings": repo["imageMappings"],
               "bundlePath": bundlePath
            }
            repo["operators"] = [op]
            sizesyaml = bundlePath + "/sizes.yaml"
            if os.path.isfile(sizesyaml):
                with open(sizesyaml, 'r', encoding='utf-8') as f:
                    sizes = yaml.safe_load(f)
            else:
                sizes = {}

        else:
            logging.critical("Config entry doesn't specify either a Git repo or a generation command")
            sys.exit(1)

        # Loop through each operator in the repo identified by the config
        for operator in repo["operators"]:
            logging.info("Helm Chartifying - %s!", operator["name"])
            # Generate and return path to CSV based on bundlePath or bundles-directory
            bundlepath = getBundleManifestsPath(repo["repo_name"], operator)
            logging.info("The latest bundle path for channel is %s", bundlepath)

            csvPath = get_csv_path(repo["repo_name"], operator, branch)
            if csvPath == "":
                # Validate the bundlePath exists in config.yaml
                logging.error("Unable to find given channel: %s", operator.get("channel", "Channel not specified"))
                sys.exit(1)

            escaped_variables = operator.get("escape-template-variables", [])

            # Validate CSV exists
            if not os.path.isfile(csvPath):
                logging.critical("Unable to find CSV at given path - '%s'.", csvPath)
                sys.exit(1)

            if lint:
                # Lint the CSV
                errs = validateCSV(csvPath)
                if len(errs) > 0:
                    logging.error("CSV Validation errors detected")
                    for err in errs:
                        logging.error(err)
                    sys.exit(1)
                logging.info("CSV validated successfully!\n")
                continue

            # Get preserved files from config or set default value
            preservedFiles = operator.get("preserve_files", [])

            # If preserve_files is provided, keep only those files; otherwise, remove directory and recreate
            if preservedFiles:
                logging.info("Preserving files for operator '%s': %s", operator["name"], str(preservedFiles))

            operator_backup = backup_operator_output(destination, operator["name"])
            try:
                # Copy over all CRDs to the destination directory from the manifest folder
                addCRDs(repo["repo_name"], operator, destination, branch, preservedFiles)

                # If name is empty, fail
                helmChart = operator["name"]
                if helmChart == "":
                    logging.critical("Unable to generate helm chart without a name.")
                    sys.exit(1)

                logging.info("Creating helm chart: '%s' ...", operator["name"])
                # Template Helm Chart Directory from 'chart-templates'
                logging.info("Templating helm chart '%s' ...", operator["name"])

                # Creates a helm chart template
                templateHelmChart(destination, operator["name"], preservedFiles)
                logging.info("Helm chart template created successfully.\n")

                # Generate the Chart.yaml file based off of the CSV
                helmChart = os.path.join(destination, "charts", "toggle", operator["name"])
                logging.info("Filling Chart.yaml for helm chart '%s' ...", operator["name"])
                fillChartYaml(helmChart, operator["name"], csvPath)
                logging.info("Chart.yaml filled successfully.\n")

                # Add all basic resources to the helm chart from the CSV
                logging.info("Adding Resources from CSV to helm chart '%s' ...", operator["name"])
                extract_csv_resources(helmChart, csvPath)
                copy_additional_resources(helmChart, csvPath, branch)

                # In ACM 2.12+ we need to handle webhooks for components, so it's necessary to verify if any webhook paths
                # are available and include manifest files for processing.
                webhook_paths = operator.get("webhook_paths", [])
                if webhook_paths is not None:
                    for path in webhook_paths:
                        copy_webhook_configuration_manifests(helmChart, os.path.join(SCRIPT_DIR, "tmp", repo_name, path))

                escape_template_variables(helmChart, escaped_variables, branch)
                logging.info("Resources added from CSV successfully.\n")

                if not skipOverrides:
                    logging.info("Adding Overrides to helm chart '%s' (set --skipOverrides=true to skip) ...", operator["name"])
                    chart_errors = injectRequirements(helmChart, operator, sizes, branch)
                    errors.extend(chart_errors)
                    if chart_errors:
                        restore_operator_output(destination, operator["name"], operator_backup)
                        logging.warning(
                            "Overrides failed with %d image mapping error(s) for helm chart '%s'.",
                            len(chart_errors),
                            operator["name"],
                        )
                    else:
                        logging.info("Overrides added to helm chart '%s' successfully.", operator["name"])
            finally:
                cleanup_operator_backup(operator_backup)

    logging.info("All repositories and operators completed processing.")

    logging.info("Performing cleanup...")
    shutil.rmtree((os.path.join(os.path.dirname(os.path.realpath(__file__)), "tmp")), ignore_errors=True)
    logging.info("Cleanup completed.")

    if errors:
        logging.critical("Errors encountered during chart generation:")
        for err in errors:
            logging.error(err)
        sys.exit(1)
    else:
        logging.info("All repositories and operators processed successfully.")

    logging.info("Script execution completed.")

if __name__ == "__main__":
    main()
