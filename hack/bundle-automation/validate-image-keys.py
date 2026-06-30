#!/usr/bin/env python3
# Copyright (c) 2026 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
# Assumes: Python 3.6+

"""
Validate that all image keys required by operator charts exist in the
corresponding operator bundle extras file.

This prevents installation failures caused by missing image override keys
in the bundle's CSV environment variables.

See: https://redhat.atlassian.net/browse/ACM-31185
"""

import argparse
import json
import os
import re
import sys
import logging
import yaml
import glob
from pathlib import Path

try:
    import coloredlogs
    coloredlogs.install(level='INFO')
except ImportError:
    logging.basicConfig(level=logging.INFO)


def extract_required_image_keys(operator_path):
    """
    Extract all imageOverride keys from chart values.yaml files

    Args:
        operator_path: Path to the operator repo (backplane-operator or multiclusterhub-operator)

    Returns:
        dict: Mapping of image_key -> [list of components that use it]
    """
    required_keys = {}
    charts_pattern = os.path.join(operator_path, "pkg/templates/charts/toggle/*/values.yaml")

    logging.debug(f"Searching for charts in: {charts_pattern}")

    for values_file in glob.glob(charts_pattern):
        component = Path(values_file).parent.name
        logging.debug(f"Processing chart: {component}")

        try:
            with open(values_file, encoding='utf-8') as f:
                data = yaml.safe_load(f)
                if data and 'global' in data and 'imageOverrides' in data['global']:
                    image_overrides = data['global']['imageOverrides']
                    if image_overrides:  # Check if not None
                        for key in image_overrides.keys():
                            if key:  # Skip empty keys
                                if key not in required_keys:
                                    required_keys[key] = []
                                required_keys[key].append(component)
        except (UnicodeDecodeError, yaml.YAMLError) as e:
            logging.error(f"Failed to read or parse YAML file {values_file}: {e}")
            sys.exit(1)
        except Exception as e:
            logging.error(f"Unexpected error reading {values_file}: {e}")
            sys.exit(1)

    return required_keys


def auto_detect_version(bundle_path):
    """
    Auto-detect version from bundle extras directory

    Args:
        bundle_path: Path to the bundle repo

    Returns:
        str: Detected version (e.g., "2.17.0") or None if not found
    """
    extras_dir = Path(bundle_path) / "extras"
    if not extras_dir.exists():
        return None

    # Find all JSON files matching version pattern (x.y.z.json)
    version_pattern = re.compile(r'^(\d+)\.(\d+)\.(\d+)\.json$')

    versions = []
    for file in extras_dir.glob('*.json'):
        match = version_pattern.match(file.name)
        if match:
            # Store as tuple of integers for proper semantic version sorting
            version_tuple = (int(match.group(1)), int(match.group(2)), int(match.group(3)))
            version_str = f"{match.group(1)}.{match.group(2)}.{match.group(3)}"
            versions.append((version_tuple, version_str))

    if not versions:
        return None

    # Sort by semantic version (tuple comparison) and return the latest
    versions.sort(reverse=True, key=lambda x: x[0])
    return versions[0][1]


def get_bundle_image_keys(bundle_path, version):
    """
    Get all image-key entries from bundle extras JSON

    Args:
        bundle_path: Path to the bundle repo (mce-operator-bundle or acm-operator-bundle)
        version: Version string (e.g., "2.17.0")

    Returns:
        tuple: (set of valid image keys, dict of placeholder keys)
    """
    extras_file = Path(bundle_path) / "extras" / f"{version}.json"
    placeholder_digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

    if not extras_file.exists():
        logging.error(f"Bundle extras file not found: {extras_file}")
        logging.error(f"Make sure the bundle repo is checked out at: {bundle_path}")
        return set(), {}

    logging.debug(f"Reading bundle extras from: {extras_file}")

    try:
        with open(extras_file, encoding='utf-8') as f:
            data = json.load(f)
            valid_keys = set()
            placeholder_keys = {}

            for img in data:
                key = img.get('image-key')
                digest = img.get('image-digest', '')

                if key:
                    if digest == placeholder_digest:
                        # Track placeholder entries
                        placeholder_keys[key] = {
                            'image-name': img.get('image-name', 'unknown'),
                            'image-remote': img.get('image-remote', 'unknown')
                        }
                    else:
                        # Only count as valid if it has a real digest
                        valid_keys.add(key)

            logging.debug(f"Found {len(valid_keys)} valid image keys in bundle")
            if placeholder_keys:
                logging.debug(f"Found {len(placeholder_keys)} placeholder entries")

            return valid_keys, placeholder_keys
    except (json.JSONDecodeError, UnicodeDecodeError) as e:
        logging.error(f"Failed to read or parse JSON file {extras_file}: {e}")
        sys.exit(1)
    except Exception as e:
        logging.error(f"Unexpected error reading {extras_file}: {e}")
        sys.exit(1)


def validate(operator_path, bundle_path, version):
    """
    Validate that all required image keys exist in the bundle

    Args:
        operator_path: Path to operator repo
        bundle_path: Path to bundle repo
        version: Version string

    Returns:
        bool: True if validation passes, False otherwise
    """
    operator_name = Path(operator_path).name
    bundle_name = Path(bundle_path).name

    logging.info("Validating image keys:")
    logging.info(f"  Operator: {operator_name} ({operator_path})")
    logging.info(f"  Bundle:   {bundle_name} ({bundle_path})")
    logging.info(f"  Version:  {version}\n")

    required = extract_required_image_keys(operator_path)
    available, placeholder_keys = get_bundle_image_keys(bundle_path, version)

    if not required:
        logging.error("No image keys found in operator charts")
        return False

    if not available and not placeholder_keys:
        logging.error("No image keys found in bundle extras")
        return False

    # Check for completely missing keys
    all_bundle_keys = available | set(placeholder_keys.keys())
    missing_keys = set(required.keys()) - all_bundle_keys

    # Check for placeholder keys that are required
    required_placeholders = set(required.keys()) & set(placeholder_keys.keys())

    has_errors = False

    if missing_keys:
        has_errors = True
        logging.error("❌ VALIDATION FAILED: Missing image keys in bundle extras")
        logging.error(f"   Location: {bundle_path}/extras/{version}.json\n")
        logging.error("Missing keys and their components:")
        for key in sorted(missing_keys):
            components = ', '.join(required[key])
            logging.error(f"  - {key} (used by: {components})")

    if required_placeholders:
        has_errors = True
        logging.error("\n⚠️  PLACEHOLDER IMAGES DETECTED: Images with placeholder digest")
        logging.error("   These image keys exist but have not been built yet\n")
        logging.error("Placeholder keys and their components:")
        for key in sorted(required_placeholders):
            components = ', '.join(required[key])
            img_info = placeholder_keys[key]
            logging.error(f"  - {key} (used by: {components})")
            logging.error(f"    Image: {img_info['image-remote']}/{img_info['image-name']}")
            logging.error("    Digest: sha256:0000...0000 (PLACEHOLDER)")

    if has_errors:
        logging.error("\nTo fix this issue:")
        if missing_keys:
            logging.error(f"  1. Add the missing image entries to {bundle_name}/extras/{version}.json")
        if required_placeholders:
            logging.error("  2. Build and publish the placeholder images")
            logging.error(f"  3. Update {bundle_name}/extras/{version}.json with real image digests")
        logging.error("  4. Ensure the CSV includes OPERAND_IMAGE_* or RELATED_IMAGE_* environment variables")
        logging.error("\nSee: https://redhat.atlassian.net/browse/ACM-31185")
        return False

    logging.info(f"✅ SUCCESS: All {len(required)} required image keys are present in bundle")
    logging.info(f"   Bundle contains {len(available)} total images")
    if placeholder_keys:
        unused_placeholders = set(placeholder_keys.keys()) - set(required.keys())
        if unused_placeholders:
            logging.info(f"   Note: {len(unused_placeholders)} unused placeholder(s) exist in bundle (not required by operator)")
    return True


def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(
        description='Validate image keys between operator and bundle repos',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
Examples:
  # Validate from operator repo (auto-detects version)
  cd ~/stolostron/backplane-operator
  %(prog)s --bundle ~/stolostron/mce-operator-bundle

  # Validate ACM
  cd ~/stolostron/multiclusterhub-operator
  %(prog)s --bundle ~/stolostron/acm-operator-bundle

  # Specify custom operator path
  %(prog)s --operator ~/stolostron/backplane-operator \\
           --bundle ~/stolostron/mce-operator-bundle

  # Use environment variables
  export BUNDLE=~/stolostron/mce-operator-bundle
  %(prog)s
        '''
    )

    parser.add_argument(
        '--operator',
        help='Path to operator repo (default: current directory)',
        default=os.environ.get('OPERATOR_PATH', '.')
    )
    parser.add_argument(
        '--bundle',
        help='Path to bundle repo (mce-operator-bundle or acm-operator-bundle)',
        default=os.environ.get('BUNDLE')
    )
    parser.add_argument(
        '--version',
        help='Version to validate against (default: auto-detect from bundle)',
        default=None
    )
    parser.add_argument(
        '--debug',
        action='store_true',
        help='Enable debug logging'
    )

    args = parser.parse_args()

    if args.debug:
        logging.getLogger().setLevel(logging.DEBUG)

    if not args.bundle:
        logging.error("--bundle is required (or set BUNDLE environment variable)")
        sys.exit(1)

    operator_path = Path(args.operator).expanduser()
    bundle_path = Path(args.bundle).expanduser()

    if not operator_path.exists():
        logging.error(f"Operator path does not exist: {operator_path}")
        sys.exit(1)

    if not bundle_path.exists():
        logging.error(f"Bundle path does not exist: {bundle_path}")
        sys.exit(1)

    # Auto-detect version if not provided
    version = args.version
    if not version:
        version = auto_detect_version(bundle_path)
        if not version:
            logging.error(f"Could not auto-detect version from {bundle_path}/extras/")
            logging.error("Please specify --version explicitly")
            sys.exit(1)
        logging.info(f"Auto-detected bundle version: {version}")

    success = validate(operator_path, bundle_path, version)
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
