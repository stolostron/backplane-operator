#!/usr/bin/env python3
# Copyright (c) 2025 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
# Assumes: Python 3.6+

import sys
import logging

REQUIRED_BUNDLE_FIELD_DESCRIPTION = "This is necessary for generating tool-specific bundles."

def get_required_config_value(config, key, description=REQUIRED_BUNDLE_FIELD_DESCRIPTION):
    """
    Retrieves a required configuration value. If the value is not found,
    logs an error and exits the program.
    :param config: The configuration dictionary.
    :param key: The key in the configuration to retrieve.
    :param description: A description of the required field for logging.
    :return: The value from the configuration if it exists.
    """
    value = config.get(key)
    if not value:
        logging.critical(
            f"Missing required field '{key}'. "
            f"{description} Please provide a valid {key}."
        )
        sys.exit(1)
    return value