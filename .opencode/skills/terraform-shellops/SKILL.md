---
name: terraform-shellops
description: >
  Terraform IaC patterns: module design, remote state management, workspace strategies,
  variable/output conventions, plan-before-apply discipline, drift detection, import
  existing resources, GitOps with Atlantis/Terraform Cloud, and Axiom traceability
  integration. Load this skill when writing, reviewing, or operating Terraform infrastructure.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - aws-cli-shellops
    - kubernetes-shellops
    - cloud-engineer-axiom
    - sre-ops-axiom
    - hardening-security-axiom
    - version-pinning-axiom
tags:
  vertical: [devops, iac, cloud, terraform]
  category: infrastructure-as-code
  core: false
---

# Terraform — Axiom Integration Skill

> **"Plan before apply. Always. No exceptions in production."**
> **"State is sacred. Remote, locked, and backed up."**
> **"Modules are reusable contracts, not copy-paste convenience."**

This skill provides production-grade Terraform IaC patterns for Axiom workflows.
It covers repository structure, module design, state management, CI/CD integration,
and how to produce traceable evidence for every infrastructure change.

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
- Writing new Terraform configurations or modules
- Reviewing Terraform code for production readiness
- Setting up remote state backends (S3+DynamoDB, Terraform Cloud, GCS)
- Implementing Terraform CI/CD with Atlantis or GitHub Actions
- Importing existing resources into Terraform state
- Debugging `terraform plan` or `terraform apply` failures
- Running drift detection or state reconciliation
- Performing destructive operations (destroy, state rm)

---

## Non-Negotiables

1. **`terraform plan` before every `apply`.** Save the plan file and review it.
   In CI: `terraform plan -out=tfplan && terraform show tfplan`. Never auto-apply
   without human review in production.

2. **Remote state with locking.** Use S3+DynamoDB, Terraform Cloud, or GCS backend.
   Never use local state in shared environments. State lock prevents concurrent runs.

3. **Pin provider and module versions.** Use `~>` or exact versions. Never use
   latest/unbounded versions. Update versions in a dedicated PR with test plan.

4. **No secrets in `.tfvars` files committed to git.** Use environment variables,
   Vault, AWS Secrets Manager, or GitHub Actions secrets. Mark sensitive outputs.

5. **`terraform validate` and `terraform fmt` in pre-commit.** Run before every push.
   Formatting is not optional — it aids diffs and reviews.

6. **`terraform destroy` requires human approval.** Never automate destroy for production.
   Flag `required_human_review: true` in work items involving destroy.

---

## Repository Structure

### Recommended Layout

```
infrastructure/
├── modules/                    # Reusable modules (versioned via git tags)
│   ├── vpc/
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   ├── outputs.tf
│   │   └── README.md           # Module interface documentation
│   ├── eks-cluster/
│   └── rds-postgres/
│
├── environments/               # Environment-specific root configurations
│   ├── _shared/                # Shared locals and data sources
│   ├── dev/
│   │   ├── main.tf
│   │   ├── variables.tf
│   │   ├── terraform.tfvars    # Non-sensitive values only
│   │   └── backend.tf
│   ├── staging/
│   └── production/
│
├── .terraform-version          # tfenv version pin
├── .tflint.hcl                 # Linting rules
└── atlantis.yaml               # Atlantis CI config (if using Atlantis)
```

### Root Module Pattern

```hcl
# environments/production/main.tf

terraform {
  required_version = "~> 1.8"   # Pin major.minor; allow patch updates

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.50"        # Pin to minor; review patches
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30"
    }
  }

  backend "s3" {
    bucket         = "my-terraform-state"
    key            = "production/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "terraform-lock"
  }
}

# Module invocation — always use pinned versions
module "vpc" {
  source  = "../../modules/vpc"
  # OR for registry modules:
  # source  = "terraform-aws-modules/vpc/aws"
  # version = "~> 5.8"

  name             = "prod-vpc"
  cidr             = var.vpc_cidr
  azs              = var.availability_zones
  private_subnets  = var.private_subnet_cidrs
  public_subnets   = var.public_subnet_cidrs

  tags = local.common_tags
}
```

---

## Module Design Patterns

### Module Interface Contract

Every module MUST have:
- `variables.tf` — all inputs with types and descriptions
- `outputs.tf` — all outputs needed by consumers
- `README.md` — usage example, inputs table, outputs table

```hcl
# modules/rds-postgres/variables.tf

variable "identifier" {
  description = "The RDS instance identifier. Must be unique within the account."
  type        = string
}

variable "instance_class" {
  description = "RDS instance class (e.g., db.t3.micro, db.r6g.large)."
  type        = string
  default     = "db.t3.micro"
}

variable "database_name" {
  description = "Name of the initial database to create."
  type        = string
}

variable "master_password" {
  description = "Master password. Sensitive — inject from Vault or Secrets Manager."
  type        = string
  sensitive   = true    # Marks as sensitive: never shown in plan output
}

variable "deletion_protection" {
  description = "Enable deletion protection. Set to true in production."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags to apply to all resources."
  type        = map(string)
  default     = {}
}
```

```hcl
# modules/rds-postgres/outputs.tf

output "endpoint" {
  description = "The RDS instance endpoint."
  value       = aws_db_instance.this.endpoint
}

output "port" {
  description = "The RDS instance port."
  value       = aws_db_instance.this.port
}

output "instance_id" {
  description = "The RDS instance ID."
  value       = aws_db_instance.this.id
}

output "arn" {
  description = "The RDS instance ARN."
  value       = aws_db_instance.this.arn
}
```

---

## State Management

### Backend Setup (S3 + DynamoDB)

```hcl
# Create state bucket (bootstrap — run once manually)
# Never manage state bucket via Terraform (chicken-egg problem)

# backend.tf
terraform {
  backend "s3" {
    bucket         = "my-org-terraform-state"
    key            = "<environment>/terraform.tfstate"  # Pass via: terraform init -backend-config="key=$ENV/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true                           # Required
    dynamodb_table = "terraform-state-lock"         # Prevents concurrent runs
    
    # Optional: state file versioning for recovery
    # Versioning on the bucket handles this
  }
}
```

### State Operations (Evidence Required)

```bash
# ALWAYS backup before state surgery
BACKUP_FILE=".memory-bank/work-items/${WORK_ITEM_ID}/tf-state-backup-$(date +%Y%m%dT%H%M%S).tfstate"
terraform state pull > "$BACKUP_FILE"
echo "State backed up to: $BACKUP_FILE"

# List all resources in state
terraform state list | tee .memory-bank/work-items/${WORK_ITEM_ID}/state-list.txt

# Show specific resource
terraform state show aws_instance.my_instance

# Move resource (e.g., module refactor)
terraform state mv aws_instance.my_instance module.compute.aws_instance.main

# Remove from state (resource deleted manually — not destroying)
terraform state rm aws_instance.orphaned_instance

# Import existing resource
terraform import aws_instance.my_instance i-1234567890abcdef0
```

---

## Variable and Locals Patterns

```hcl
# Common locals pattern (shared across modules)
locals {
  # Deterministic naming
  name_prefix = "${var.project}-${var.environment}"

  # Common tags (apply to every resource)
  common_tags = merge(
    var.additional_tags,
    {
      Project     = var.project
      Environment = var.environment
      ManagedBy   = "terraform"
      WorkItem    = var.work_item_id   # Axiom trace
    }
  )
}

# variables.tf — required non-sensitive values
variable "project" {
  description = "Project name used in resource naming."
  type        = string

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{2,30}$", var.project))
    error_message = "Project name must be lowercase, start with letter, 3-31 chars."
  }
}

variable "environment" {
  description = "Deployment environment (dev/staging/production)."
  type        = string

  validation {
    condition     = contains(["dev", "staging", "production"], var.environment)
    error_message = "Environment must be dev, staging, or production."
  }
}

variable "work_item_id" {
  description = "Axiom work item ID for traceability."
  type        = string
  default     = "unknown"
}
```

---

## Plan Workflow (CI/CD Standard)

```bash
# Standard plan → review → apply workflow

# 1. Initialize (download providers/modules)
terraform init -upgrade

# 2. Validate syntax
terraform validate

# 3. Format check
terraform fmt -check -recursive
# Auto-fix: terraform fmt -recursive

# 4. Lint (requires tflint)
tflint --recursive

# 5. Generate plan (save for apply step)
terraform plan \
  -out=tfplan \
  -var="work_item_id=${WORK_ITEM_ID}" \
  2>&1 | tee .memory-bank/work-items/${WORK_ITEM_ID}/terraform-plan.txt

# 6. Human-readable plan summary
terraform show tfplan 2>&1 | tee .memory-bank/work-items/${WORK_ITEM_ID}/terraform-plan-readable.txt

# 7. REVIEW — check for unexpected destroys
# grep for "will be destroyed" in plan output
grep -E "(will be destroyed|must be replaced)" \
  .memory-bank/work-items/${WORK_ITEM_ID}/terraform-plan.txt || echo "No destroys in plan"

# 8. Apply the saved plan (not a new plan)
terraform apply tfplan 2>&1 | tee .memory-bank/work-items/${WORK_ITEM_ID}/terraform-apply.txt

# 9. Capture final state for evidence
terraform state list > .memory-bank/work-items/${WORK_ITEM_ID}/post-apply-state-list.txt
```

---

## Drift Detection

```bash
# Run refresh + plan to detect drift
terraform plan -refresh-only 2>&1 | tee /tmp/drift-check.txt

# Check if there are any changes
if grep -q "No changes." /tmp/drift-check.txt; then
  echo "✅ No drift detected"
else
  echo "⚠️  Drift detected — review /tmp/drift-check.txt"
  grep -E "will be (updated|created|destroyed)" /tmp/drift-check.txt
fi
```

---

## Common Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `terraform apply` without `-out=tfplan` | Applies new plan that differs from what you reviewed | Always `-out=tfplan`, then `apply tfplan` |
| `version = "latest"` for providers/modules | Surprise breaking changes; non-reproducible | Pin with `~>` or exact version |
| Local state in shared environments | Concurrent runs corrupt state; lost on dev machine | Remote backend + DynamoDB lock |
| Secrets in `terraform.tfvars` committed to git | Credential leak | `TF_VAR_secret` env vars or Vault |
| Resources not tagged with `ManagedBy=terraform` | Lost track of what Terraform owns | `common_tags` with `ManagedBy` |
| Giant monolithic root module | Hard to plan; blast radius is entire infra | Split by lifecycle: network/compute/data |
| `count` instead of `for_each` for resources | Index-based deletion reorders all resources | Use `for_each` with stable string keys |
| No `deletion_protection = true` in prod | Accidental destroy wipes production DB | Set it; require explicit override |
| `terraform destroy` in CI auto-approve | Irreversible production deletion | Never auto-approve destroy; require gate |
| No `backend.tf` version pinning | Terraform cloud migration breaks unexpectedly | Pin Terraform version in `required_version` |

---

## Axiom Evidence Template

```markdown
## Terraform Evidence: <description>

**Work Item**: ${WORK_ITEM_ID}
**Environment**: production
**Operator**: <identity>
**Date**: $(date -u +%Y-%m-%dT%H:%M:%SZ)

### Pre-Apply State
- state_list: terraform-pre-apply-state.txt
- caller_identity: aws-identity.json

### Plan Summary
- plan_file: terraform-plan.txt
- destroys: 0 (verified with grep)
- creates: 3
- updates: 1

### Apply Output
- apply_log: terraform-apply.txt
- status: SUCCESS

### Post-Apply Verification
- [ ] Resources exist in AWS Console
- [ ] Application health checks pass
- [ ] No unexpected state drift

axiom:trace work_item=${WORK_ITEM_ID} impl=infrastructure/environments/production ops=.axiom/runbooks/terraform-deploy.md evidence=.memory-bank/work-items/${WORK_ITEM_ID}/terraform-apply.txt
```

---

## axiom:trace

`axiom:trace work_item=terraform-shellops-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/terraform-shellops/SKILL.md evidence= doc= ops= commit=`
