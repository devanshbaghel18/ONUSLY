#!/usr/bin/env bash
# ==============================================================================
# ONUSLY — Mumbai (ap-south-1) EC2 Automated Setup Wrapper
# Target OS: Amazon Linux 2023
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default active DynamoDB table region is eu-north-1 (override via AWS_REGION if running a Mumbai replica)
export AWS_REGION="${AWS_REGION:-eu-north-1}"
export DYNAMODB_TABLE="${DYNAMODB_TABLE:-Onusly}"

echo ">>> Delegating EC2 bootstrap to deploy/setup_ec2.sh (AWS_REGION=${AWS_REGION})..."
exec bash "${SCRIPT_DIR}/setup_ec2.sh" "$@"
