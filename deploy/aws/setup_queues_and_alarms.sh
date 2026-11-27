#!/usr/bin/env bash
# ==============================================================================
# ONUSLY — Idempotent AWS SQS Provisioning & CloudWatch Alarm Setup (eu-north-1)
# Provisions durable SQS queues (proof-events, approval-events + DLQs) and basic
# EC2/CW alarms. Run manually by an operator with least-privilege AWS CLI access.
# ==============================================================================

set -euo pipefail

REGION="${AWS_REGION:-eu-north-1}"
PROOF_QUEUE="${PROOF_QUEUE_NAME:-proof-events}"
APPROVAL_QUEUE="${APPROVAL_QUEUE_NAME:-approval-events}"
PROOF_DLQ="${PROOF_QUEUE}-dlq"
APPROVAL_DLQ="${APPROVAL_QUEUE}-dlq"
INSTANCE_ID="${ONUSLY_EC2_INSTANCE_ID:-}"

echo ">>> Provisioning SQS queues in ${REGION}..."

ensure_queue() {
  local name="$1"
  local url
  if url="$(aws sqs get-queue-url --region "$REGION" --queue-name "$name" --query 'QueueUrl' --output text 2>/dev/null)"; then
    echo "$url"
    return 0
  fi
  aws sqs create-queue \
    --region "$REGION" \
    --queue-name "$name" \
    --attributes '{"MessageRetentionPeriod":"1209600","VisibilityTimeout":"60"}' \
    --query 'QueueUrl' --output text
}

bind_dlq() {
  local main_url="$1"
  local dlq_url="$2"
  local dlq_arn
  dlq_arn="$(aws sqs get-queue-attributes --region "$REGION" --queue-url "$dlq_url" --attribute-names QueueArn --query 'Attributes.QueueArn' --output text)"
  aws sqs set-queue-attributes \
    --region "$REGION" \
    --queue-url "$main_url" \
    --attributes "{\"RedrivePolicy\":\"{\\\"deadLetterTargetArn\\\":\\\"${dlq_arn}\\\",\\\"maxReceiveCount\\\":\\\"5\\\"}\"}"
}

PROOF_DLQ_URL="$(ensure_queue "$PROOF_DLQ")"
APPROVAL_DLQ_URL="$(ensure_queue "$APPROVAL_DLQ")"
PROOF_URL="$(ensure_queue "$PROOF_QUEUE")"
APPROVAL_URL="$(ensure_queue "$APPROVAL_QUEUE")"

bind_dlq "$PROOF_URL" "$PROOF_DLQ_URL"
bind_dlq "$APPROVAL_URL" "$APPROVAL_DLQ_URL"

echo "[+] Add these lines to /etc/onusly/onusly.env on EC2:"
echo "PROOF_EVENTS_QUEUE_URL=${PROOF_URL}"
echo "APPROVAL_EVENTS_QUEUE_URL=${APPROVAL_URL}"

if [ -n "$INSTANCE_ID" ]; then
  echo ">>> Configuring CloudWatch EC2 Status & CPU Alarms for ${INSTANCE_ID}..."
  aws cloudwatch put-metric-alarm \
    --region "$REGION" \
    --alarm-name "onusly-ec2-status-check-failed" \
    --alarm-description "ONUSLY EC2 instance status check failed" \
    --namespace "AWS/EC2" \
    --metric-name "StatusCheckFailed" \
    --dimensions "Name=InstanceId,Value=${INSTANCE_ID}" \
    --statistic Maximum \
    --period 60 \
    --evaluation-periods 2 \
    --threshold 1 \
    --comparison-operator GreaterThanOrEqualToThreshold

  aws cloudwatch put-metric-alarm \
    --region "$REGION" \
    --alarm-name "onusly-ec2-high-cpu" \
    --alarm-description "ONUSLY EC2 sustained high CPU utilization (>85%)" \
    --namespace "AWS/EC2" \
    --metric-name "CPUUtilization" \
    --dimensions "Name=InstanceId,Value=${INSTANCE_ID}" \
    --statistic Average \
    --period 300 \
    --evaluation-periods 3 \
    --threshold 85 \
    --comparison-operator GreaterThanOrEqualToThreshold
  echo "[+] CloudWatch EC2 alarms configured."
else
  echo "[!] Set ONUSLY_EC2_INSTANCE_ID=<i-...> to also create EC2 CloudWatch alarms."
fi
