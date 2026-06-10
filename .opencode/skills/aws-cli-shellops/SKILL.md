---
name: aws-cli-shellops
description: >
  AWS CLI patterns, IAM least-privilege enforcement, common service operations (EC2, S3,
  EKS, RDS, Lambda, CloudWatch, STS), credential management, and Axiom traceability
  integration for AWS-backed infrastructure work. Load this skill when writing Terraform,
  automation scripts, runbooks, or any task that touches AWS resources via CLI or SDK.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - terraform-shellops
    - kubernetes-shellops
    - sre-ops-axiom
    - hardening-security-axiom
    - cloud-engineer-axiom
tags:
  vertical: [devops, sre, cloud, aws]
  category: cloud-operations
  core: false
---

# AWS CLI — Axiom Integration Skill

> **"No hardcoded credentials. Ever."**
> **"Least privilege IAM — if the script doesn't need it, the role doesn't have it."**
> **"Every AWS action that changes state must produce traceable evidence."**

This skill provides production-grade AWS CLI patterns for DevOps and SRE work within
Axiom. It covers credential hygiene, per-service command patterns, output formats,
and how to produce evidence that satisfies Axiom verification gates.

---


## Scope & Prerequisites

> **Scope**: These patterns are tuned for the Dexdat infrastructure. Values in `<angle brackets>` are placeholders — replace with your environment. Values in `${VAR}` require the named environment variable to be set.

```bash
# Verify required tools
command -v jq      || echo 'MISSING: brew install jq'
command -v kubectl || echo 'MISSING: https://kubernetes.io/docs/tasks/tools/'
command -v aws     || echo 'MISSING: https://awscli.amazonaws.com'
# Set required environment variables
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text 2>/dev/null)
export WORK_ITEM_ID="${WORK_ITEM_ID:-$(date +%Y%m%d)-ops}"   # Axiom work item ID
export CLUSTER_NAME="${CLUSTER_NAME:-}"   # aws eks list-clusters --region $AWS_REGION
```

---

## Activation

Load this skill when:
- Writing or reviewing AWS CLI commands in scripts, runbooks, or Terraform
- Setting up IAM roles, policies, or permission boundaries
- Operating EKS clusters, EC2 instances, RDS databases, or Lambda functions
- Creating S3 operations (sync, cp, presigned URLs, lifecycle)
- Working with CloudWatch Logs, Metrics, or Alarms
- Running `aws sts assume-role` for cross-account operations
- Capturing evidence of AWS state before/after infrastructure changes

---

## Non-Negotiables

1. **No hardcoded credentials.** Use IAM roles, instance profiles, or `aws configure sso`.
   Never put `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` in code, scripts, or `.env` files
   committed to git. If found: stop, rotate immediately, call `@security-review-axiom`.

2. **Always specify `--region`.** Never rely on ambient region. Explicitly pass
   `--region $AWS_REGION` or export `AWS_DEFAULT_REGION` at script top.

3. **Least-privilege IAM.** Every automation role needs only what the script actually calls.
   Run `aws iam simulate-principal-policy` to verify before deploying.

4. **`--dry-run` before destructive operations.** EC2, S3 delete, and many mutating operations
   support `--dry-run`. Always use it first in CI/CD and runbooks.

5. **Capture `--output json` for evidence.** Machine-readable output is required for
   Axiom evidence bundles. Never capture `--output table` as verification evidence.

6. **Profile isolation.** Use named profiles (`~/.aws/config`) to separate prod/staging/dev.
   CI/CD uses IAM roles, not static keys.

---

## Credential Patterns

### Profile-Based (local development)

```bash
# Named profile setup
aws configure --profile prod-readonly
aws configure --profile dev-admin

# Use profile
export AWS_PROFILE=prod-readonly
aws s3 ls s3://my-bucket

# Or inline
aws --profile prod-readonly s3 ls s3://my-bucket
```

### IAM Role Assumption (cross-account / least privilege)

```bash
# Assume role and export creds
CREDS=$(aws sts assume-role \
  --role-arn "arn:aws:iam::123456789012:role/MyDeployRole" \
  --role-session-name "axiom-deploy-$(date +%s)" \
  --output json)

export AWS_ACCESS_KEY_ID=$(echo $CREDS | jq -r .Credentials.AccessKeyId)
export AWS_SECRET_ACCESS_KEY=$(echo $CREDS | jq -r .Credentials.SecretAccessKey)
export AWS_SESSION_TOKEN=$(echo $CREDS | jq -r .Credentials.SessionToken)
unset CREDS  # ← Required: clear raw JSON from shell memory immediately

# In CI: mask the extracted values
# echo "::add-mask::$AWS_ACCESS_KEY_ID"
# Note: prefix command with a space to suppress shell history: " export AWS_ACCESS..."

# Verify who you are
aws sts get-caller-identity --output json
```

### SSO (enterprise)

```bash
# Configure SSO
aws configure sso

# Login
aws sso login --profile my-sso-profile

# Verify
aws --profile my-sso-profile sts get-caller-identity
```

### Verify Identity Before ANY Destructive Action

```bash
# ALWAYS run this before prod operations
aws sts get-caller-identity --output json | tee /tmp/aws-identity-$(date +%s).json
# Confirm you are in the right account before continuing
```

---

## IAM Patterns

### Create Policy with Least Privilege

```bash
# Create inline policy document
cat > /tmp/policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::my-bucket/*"
    }
  ]
}
EOF

# Create managed policy
aws iam create-policy \
  --policy-name "MyAppS3ReadWrite" \
  --policy-document file:///tmp/policy.json \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/aws-policy-created.json

# Attach to role
aws iam attach-role-policy \
  --role-name MyAppRole \
  --policy-arn arn:aws:iam::123456789012:policy/MyAppS3ReadWrite
```

### Simulate Policy (verify least privilege)

```bash
# Simulate what actions are allowed
aws iam simulate-principal-policy \
  --policy-source-arn "arn:aws:iam::123456789012:role/MyAppRole" \
  --action-names "s3:GetObject" "s3:DeleteObject" "s3:PutObject" \
  --resource-arns "arn:aws:s3:::my-bucket/*" \
  --output json | tee .memory-bank/work-items/${WORK_ITEM_ID}/iam-simulation.json
```

### List Role Policies

```bash
# Attached managed policies
aws iam list-attached-role-policies --role-name MyAppRole --output json

# Inline policies
aws iam list-role-policies --role-name MyAppRole --output json

# Get inline policy detail
aws iam get-role-policy --role-name MyAppRole --policy-name MyInlinePolicy --output json
```

---

## EC2 Patterns

### Instance Management

```bash
# List instances with useful filters
aws ec2 describe-instances \
  --filters "Name=tag:Environment,Values=production" \
            "Name=instance-state-name,Values=running" \
  --query 'Reservations[].Instances[].{ID:InstanceId,Type:InstanceType,IP:PrivateIpAddress,Name:Tags[?Key==`Name`]|[0].Value}' \
  --output table

# Start/stop (dry run first)
aws ec2 stop-instances --instance-ids i-1234567890abcdef0 --dry-run
aws ec2 stop-instances --instance-ids i-1234567890abcdef0 --output json | tee /tmp/stop-evidence.json

# SSM Session (preferred over SSH — no inbound ports needed)
aws ssm start-session --target i-1234567890abcdef0
```

### Security Groups

```bash
# Audit unrestricted ingress (should return empty for prod)
aws ec2 describe-security-groups \
  --filters "Name=ip-permission.cidr,Values=0.0.0.0/0" \
  --query 'SecurityGroups[?IpPermissions[?IpRanges[?CidrIp==`0.0.0.0/0`]]].{ID:GroupId,Name:GroupName}' \
  --output json

# Add rule (always specific CIDR, never 0.0.0.0/0)
aws ec2 authorize-security-group-ingress \
  --group-id sg-12345 \
  --protocol tcp \
  --port 443 \
  --cidr 10.0.0.0/8
```

---

## S3 Patterns

### Bucket Operations

```bash
# List buckets with creation dates
aws s3api list-buckets --output json

# Check bucket policy
aws s3api get-bucket-policy --bucket my-bucket --output json 2>/dev/null || echo "No bucket policy"

# Check public access block (should all be true for internal buckets)
aws s3api get-public-access-block --bucket my-bucket --output json

# Sync with dry-run
aws s3 sync ./dist s3://my-bucket/app --dryrun
aws s3 sync ./dist s3://my-bucket/app --delete --output json 2>&1 | tee /tmp/s3-sync-evidence.json
```

### Presigned URLs (time-limited access without credentials)

```bash
# Generate presigned URL (expires in 3600 seconds)
aws s3 presign s3://my-bucket/path/to/file --expires-in 3600
```

### Lifecycle Audit

```bash
# Check encryption
aws s3api get-bucket-encryption --bucket my-bucket --output json

# Check versioning
aws s3api get-bucket-versioning --bucket my-bucket --output json

# Check logging
aws s3api get-bucket-logging --bucket my-bucket --output json
```

---

## EKS Patterns

### Cluster Access

```bash
# Update kubeconfig (required before kubectl commands)
aws eks update-kubeconfig \
  --name my-cluster \
  --region us-east-1 \
  --profile prod-readonly
  # Adds context to ~/.kube/config

# Verify context
kubectl config current-context
kubectl cluster-info

# List node groups
aws eks list-nodegroups --cluster-name my-cluster --output json

# Describe nodegroup
aws eks describe-nodegroup \
  --cluster-name my-cluster \
  --nodegroup-name my-nodes \
  --output json | tee /tmp/nodegroup-state.json
```

### Add-On Management

```bash
# List installed add-ons
aws eks list-addons --cluster-name my-cluster --output json

# Describe add-on (check version drift)
aws eks describe-addon --cluster-name my-cluster --addon-name vpc-cni --output json
```

---

## RDS Patterns

### Database Operations

```bash
# List instances
aws rds describe-db-instances \
  --query 'DBInstances[].{ID:DBInstanceIdentifier,Class:DBInstanceClass,Status:DBInstanceStatus,Engine:Engine}' \
  --output table

# Create snapshot before any schema change
aws rds create-db-snapshot \
  --db-instance-identifier my-db \
  --db-snapshot-identifier "pre-migration-$(date +%Y%m%d-%H%M%S)" \
  --output json | tee /tmp/rds-snapshot-evidence.json
# WAIT for snapshot to complete before running migration
aws rds wait db-snapshot-completed \
  --db-snapshot-identifier "pre-migration-$(date +%Y%m%d-%H%M%S)"

# Check parameter group
aws rds describe-db-parameters \
  --db-parameter-group-name my-param-group \
  --output json
```

---

## Lambda Patterns

### Invoke and Monitor

```bash
# Synchronous invoke (captures response)
aws lambda invoke \
  --function-name my-function \
  --payload '{"key":"value"}' \
  --cli-binary-format raw-in-base64-out \
  --log-type Tail \
  /tmp/lambda-response.json

# Read logs from last invocation
aws lambda get-function \
  --function-name my-function \
  --output json | jq '.Configuration.LastUpdateStatus'

# Tail recent CloudWatch logs
aws logs tail /aws/lambda/my-function --follow --since 10m
```

---

## CloudWatch Patterns

### Logs

```bash
# List log groups
aws logs describe-log-groups \
  --log-group-name-prefix "/aws/" \
  --output json

# Query logs with Insights
aws logs start-query \
  --log-group-name "/aws/lambda/my-function" \
  --start-time $(date -d '1 hour ago' +%s) \
  --end-time $(date +%s) \
  --query-string 'fields @timestamp, @message | filter @message like /ERROR/ | sort @timestamp desc | limit 20'

# Get query results
QUERY_ID="..." # from start-query output
aws logs get-query-results --query-id $QUERY_ID --output json
```

### Alarms

```bash
# List alarms in ALARM state
aws cloudwatch describe-alarms \
  --state-value ALARM \
  --output json | jq '.MetricAlarms[] | {Name:.AlarmName, Metric:.MetricName, Reason:.StateReason}'

# Create alarm with SNS notification
aws cloudwatch put-metric-alarm \
  --alarm-name "HighCPU-my-instance" \
  --alarm-description "CPU > 80% for 5 minutes" \
  --metric-name CPUUtilization \
  --namespace AWS/EC2 \
  --statistic Average \
  --period 300 \
  --threshold 80 \
  --comparison-operator GreaterThanThreshold \
  --evaluation-periods 2 \
  --alarm-actions arn:aws:sns:us-east-1:123456789012:ops-alerts
```

---

## Output Formats and Evidence Capture

```bash
# JSON (machine-readable — required for evidence)
aws ec2 describe-instances --output json | tee evidence.json

# JQ filtering (keep evidence JSON intact, display separately)
aws ec2 describe-instances --output json > evidence.json
cat evidence.json | jq '.Reservations[].Instances[].InstanceId'

# TSV for shell pipelines
aws ec2 describe-instances \
  --query 'Reservations[].Instances[].[InstanceId,InstanceType,State.Name]' \
  --output text

# Store evidence with trace marker
EVIDENCE_FILE=".memory-bank/work-items/${WORK_ITEM_ID}/aws-state-$(date +%Y%m%dT%H%M%S).json"
aws ec2 describe-instances --output json > "$EVIDENCE_FILE"
echo "axiom:trace work_item=${WORK_ITEM_ID} evidence=${EVIDENCE_FILE}"
```

---

## Axiom Integration

### Trace Pattern for AWS Operations

Every mutating AWS operation MUST produce evidence and a trace marker:

```bash
# Pattern for evidence-producing AWS operations
WORK_ITEM_ID="my-work-item-id"
EVIDENCE_DIR=".memory-bank/work-items/${WORK_ITEM_ID}"
mkdir -p "$EVIDENCE_DIR"

# 1. Capture pre-state
aws rds describe-db-instances --output json > "${EVIDENCE_DIR}/pre-state.json"

# 2. Run operation
aws rds create-db-snapshot \
  --db-instance-identifier my-db \
  --db-snapshot-identifier "snapshot-$(date +%Y%m%d)" \
  --output json > "${EVIDENCE_DIR}/snapshot-created.json"

# 3. Capture post-state
aws rds describe-db-instances --output json > "${EVIDENCE_DIR}/post-state.json"

# 4. Add trace marker
cat >> "${EVIDENCE_DIR}/verification.md" <<EOF
## AWS Evidence: RDS Snapshot
- pre_state: pre-state.json
- snapshot: snapshot-created.json
- post_state: post-state.json
- axiom:trace work_item=${WORK_ITEM_ID} ops=.axiom/runbooks/rds-snapshot.md evidence=${EVIDENCE_DIR}/snapshot-created.json
EOF
```

### Required Evidence for AWS Work Items

| Operation Type | Required Evidence |
|---|---|
| IAM changes | `iam-simulate-policy.json` + before/after policy comparison |
| Infrastructure create/delete | `pre-state.json` + `post-state.json` |
| Database migrations | `pre-snapshot.json` + migration output + health check |
| Deploy | `caller-identity.json` + before/after resource state |
| Security group changes | `before-sg.json` + `after-sg.json` + ingress audit |

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `AWS_ACCESS_KEY_ID` in scripts | Credential leak risk; git history exposure | IAM roles / SSO / instance profile |
| `--region` omitted | Wrong region surprises; silent failures | Always explicit `--region` or `AWS_DEFAULT_REGION` |
| `--output text` for evidence | Unparseable; not machine-readable | Always `--output json` for evidence |
| No dry-run before delete | Irreversible data loss | Use `--dry-run`; document skip if unsupported |
| AdministratorAccess for automation | Blast radius = entire account | Least-privilege policy per automation |
| `aws s3 rm --recursive` without backup | Data loss | Snapshot/backup + verify before delete |
| Ignoring `aws configure` output leaks | Profiles leak region/account in CI logs | Mask env vars in CI; use roles not keys |
| `aws ... | grep` for evidence | Non-deterministic; breaks on output changes | `--output json` + `jq` |

---

## Useful Aliases and Shell Config

```bash
# ~/.bashrc or ~/.zshrc additions for AWS work
alias awsid='aws sts get-caller-identity --output json'
alias awsregion='aws configure get region'

# Switch profiles quickly
awsprofile() { export AWS_PROFILE="$1"; awsid; }

# Safe EC2 stop (shows instance name first)
ec2-stop() {
  local id=$1
  aws ec2 describe-instances --instance-ids "$id" \
    --query 'Reservations[0].Instances[0].Tags[?Key==`Name`].Value | [0]' \
    --output text
  read -p "Stop this instance? (y/N) " confirm
  [[ $confirm == y ]] && aws ec2 stop-instances --instance-ids "$id" --output json
}
```

---

## axiom:trace

`axiom:trace work_item=aws-cli-shellops-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/aws-cli-shellops/SKILL.md evidence= doc= ops= commit=`
