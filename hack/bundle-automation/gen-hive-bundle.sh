#!/bin/bash

# Generates the Hive bundle using a script in the Hive operator repo.
#
# Arguments:
#
# $1 = Name of branch of commit SHA to use for the tool, and the bundle input.
# $2 = Commit SHA (or HEAD) on branch named in $1
# $3 = Pathname of the directory to contain the resulting Hive bundle.
#
# Note: The output directory is removed at the start of each run to ensure
#       a clean/consistent result.

me=$(basename "$0")

branch="$1"
commit_ish="$2"
output_dir="$3"

if [[ -z "$output_dir" ]]; then
   >&2 echo "Syntax: $me <commit_ish> <output_path>"
   exit 5
fi

hive_repo="https://github.com/openshift/hive.git"
gen_tool="hack/bundle-gen.py"  # Path within Hive's repo.

tmp_dir=$(mktemp -td "$me.XXXXXXXX")

start_cwd="$PWD"
rm -rf "$output_dir"

cd "$tmp_dir"

# Clone the Hive operator repo at specified commit/branch.

echo "Cloning/checking out Hive repo at branch/commit ($commit_ish)."
hive_repo_spot="$PWD/hive"
git clone --no-progress "$hive_repo" "hive"
rc=$?
if [[ $rc -ne 0 ]]; then
   >&2 echo "Error: Could not clone openshift/hive (rc: $rc)."
   exit 3
fi
cd hive
git fetch origin $branch
git checkout $branch
git -c advice.detachedHead=false checkout "$commit_ish"
rc=$?
if [[ $rc -ne 0 ]]; then
   >&2 echo "Error: Could not checkout branch/commit $commit_ish (rc: $rc)."
   exit 3
fi
cd ..

# Run Hive's bundle generation tool.  It puts its output in a subdirectory of $PWD
# named with pattern "hive-operator-bundle-*" so run it from a clean directory so
# we sure there will only ever be one such-named subdirectory.

if [[ ! -f "./hive/$gen_tool" ]]; then
   >&2 echo "Error: Hive's bundle_gen tool ($gen_tool) not found."
   exit 3
fi

mkdir bundle
cd bundle
echo "Running Hive bundle-gen tool ($gen_tool)."
python3 ../hive/$gen_tool --hive-repo "$hive_repo_spot" --commit "$commit_ish" --dummy-bundle "$branch" \
   --image-repo disable-image-validation

# Note: We point the bundle-gen tool at the local repo we already checked out
# since we know that it contains the Git SHA we are using for input.
rc=$?
if [[ $rc -ne 0 ]]; then
   >&2 echo "Error: Hive's bundle_gen script failed (rc: $rc)."
   exit 3
fi

# Check that an output directory was created, and copy the results into
# the output directory specified to us.

generated_bundle_dir="$PWD/hive-operator-bundle-*"

if ! ls $generated_bundle_dir > /dev/null 2>&1; then
   >&2 echo "Error: Hive's bundle_gen script didn't generate expected output directory."
   exit 3
fi

cd "$start_cwd"
mkdir -p "$output_dir"
echo "Copying generated bundle manifests to output directory."
cp -p $generated_bundle_dir/**/* $output_dir

rc=$?
if [[ $rc -ne 0 ]]; then
   >&2 echo "Error: Error copying generated bundle manifests(rc: $rc)."
   exit 3
fi

echo "Hive bundle copied to $output_dir."

rm -rf "$tmp_dir"

