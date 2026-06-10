---
name: flyte-shellops
description: >
  Flyte ML/scientific workflow platform: workflow and task authoring in Python, FlyteRemote
  execution, pod template configuration, resource management for GPU/CPU tasks, image spec
  management, cross-account S3 access, Flyte console operations, and Axiom traceability
  integration. Primary workflow platform for scientific pipelines at Dexdat.
license: MIT
compatibility: opencode
metadata:
  version: "1.0"
  created: "2026-05-20"
  primary_spec: specs/00-PRD.md
  related_skills:
    - kubernetes-shellops
    - aws-cli-shellops
    - docker-shellops
    - temporal-shellops
    - helm-shellops
tags:
  vertical: [devops, mlops, scientific, flyte, workflows]
  category: workflow-orchestration
  core: false
---

# Flyte — Axiom Integration Skill

> **"A workflow is only as reliable as its task definitions and resource requests."**
> **"Flyte workflows are reproducible by design — the same input always produces the same output."**
> **"GPU tasks need explicit resource requests. 'It'll get scheduled eventually' is not a resource plan."**

This skill covers Flyte as the primary scientific workflow platform at Dexdat.
Flyte (`flyte-core` + `flyte-pod-templates`) runs on EKS and handles ML training jobs,
protein design workflows, experiment pipelines, and other scientific compute tasks.

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
- Writing a new Flyte workflow or task in Python
- Configuring GPU resource requests for ML tasks
- Debugging a failed or stuck Flyte execution
- Adding a new image spec to `ml.infra`
- Setting up FlyteRemote for programmatic workflow submission
- Reviewing cross-account S3 bucket access for workflow data
- Configuring pod templates for specialized hardware (EFA, GPU)
- Running `pyflyte run` or `pyflyte register` commands

---

## Non-Negotiables

1. **Resource requests are required on every task.** Every `@task` must declare
   `requests=Resources(cpu=..., mem=...)`. Tasks without resource requests compete
   with other cluster workloads unpredictably and get OOMKilled.

2. **GPU tasks must specify `accelerator=`** to land on the right node pool (g6e
   for L40S, lambda nodes for H100). Without this, GPU tasks may schedule on CPU nodes
   and fail at runtime.

3. **Use versioned image specs.** Never use `:latest` in Flyte image specs. Images
   must be pinned to a SHA or semantic version for workflow reproducibility.

4. **Workflow inputs are typed.** All `@workflow` and `@task` inputs/outputs must have
   Python type annotations. Untyped interfaces break Flyte's static analysis and
   make debugging much harder.

5. **Test locally before registering.** Use `pyflyte run --local` or unit tests with
   mock task execution before `pyflyte register` to a cluster.

---

## Task and Workflow Authoring

### Basic Task + Workflow

```python
# workflows/protein_design/workflow.py
from flytekit import task, workflow, Resources, ImageSpec
from flytekit.types.file import FlyteFile
from typing import List

# Image spec — references a specific versioned container image
# Defined in ml.infra and built via GitHub Actions → ECR
PROTEIN_IMAGE = ImageSpec(
    name="protein-design",
    registry="${AWS_ACCOUNT_ID}.dkr.ecr.us-east-1.amazonaws.com",
    tag="v1.2.3",                   # Always pinned — never :latest
    packages=["flytekit", "biopython", "torch"],
)

@task(
    container_image=PROTEIN_IMAGE,
    requests=Resources(cpu="4", mem="16Gi"),        # Required
    limits=Resources(cpu="8", mem="32Gi"),
    cache=True,                                      # Cache task outputs by input hash
    cache_version="v1",
    retries=2,
)
def preprocess_sequence(sequence: str) -> List[float]:
    """Preprocess protein sequence into features."""
    # ... implementation
    return features

@task(
    container_image=PROTEIN_IMAGE,
    requests=Resources(cpu="8", mem="64Gi", gpu="1"),   # GPU request
    limits=Resources(cpu="16", mem="128Gi", gpu="1"),
    accelerator=GPUAccelerator("nvidia-l40s"),           # Target g6e nodes
    cache=True,
    cache_version="v1",
    timeout=timedelta(hours=4),
)
def run_structure_prediction(features: List[float]) -> FlyteFile:
    """Run protein structure prediction using GPU."""
    # ... implementation
    return FlyteFile(path="/tmp/structure.pdb")

@workflow
def protein_design_workflow(sequence: str) -> FlyteFile:
    """End-to-end protein design workflow.
    
    axiom:trace work_item=${WORK_ITEM_ID} impl=workflows/protein_design/workflow.py
    """
    features = preprocess_sequence(sequence=sequence)
    structure = run_structure_prediction(features=features)
    return structure
```

### Map Tasks (parallel execution)

```python
from flytekit import map_task

@task(
    requests=Resources(cpu="2", mem="8Gi"),
    cache=True, cache_version="v1",
)
def analyze_sample(sample_id: str) -> dict:
    """Analyze a single sample."""
    return results

@workflow
def batch_analysis_workflow(sample_ids: List[str]) -> List[dict]:
    """Process all samples in parallel."""
    return map_task(analyze_sample)(sample_id=sample_ids)
    # Flyte schedules these concurrently respecting cluster capacity
```

---

## Resource Configuration for Dexdat Hardware

```python
from flytekit import Resources, task
from flytekit.extras.accelerators import GPUAccelerator

# Standard CPU task
@task(requests=Resources(cpu="2", mem="8Gi"), limits=Resources(cpu="4", mem="16Gi"))

# GPU task — NVIDIA L40S (g6e instances on EKS)
@task(
    requests=Resources(cpu="8", mem="64Gi", gpu="1"),
    accelerator=GPUAccelerator("nvidia-l40s"),
)

# Multi-GPU task — H100 (Lambda Labs cluster)
@task(
    requests=Resources(cpu="32", mem="256Gi", gpu="8"),
    accelerator=GPUAccelerator("nvidia-h100"),
)

# High-memory CPU task (e.g. genomics, large sequence models)
@task(requests=Resources(cpu="16", mem="256Gi"), limits=Resources(cpu="32", mem="512Gi"))

# EFA-enabled task (for multi-node distributed training)
@task(
    requests=Resources(cpu="64", mem="512Gi", gpu="8"),
    accelerator=GPUAccelerator("nvidia-h100"),
    pod_template_name="efa-enabled",   # References flyte-pod-templates chart
)
```

---

## FlyteRemote (programmatic execution)

```python
# Execute workflows programmatically (e.g. from a lab automation script)
from flytekit.remote import FlyteRemote
from flytekit.configuration import Config, PlatformConfig

remote = FlyteRemote(
    config=Config(
        platform=PlatformConfig(
            endpoint="flyte.houston.dexdat.ai",
            auth_mode="pkce",           # Or "client_credentials" for service accounts
        )
    ),
    default_project="my-project",
    default_domain="production",
)

# Register (if not already registered via CI)
remote.register_workflow(protein_design_workflow, version="v1.2.3")

# Execute
execution = remote.execute(
    remote.fetch_workflow(
        project="my-project",
        domain="production",
        name="workflows.protein_design.workflow.protein_design_workflow",
        version="v1.2.3",
    ),
    inputs={"sequence": "MKTIIALSYIFCLVFA"},
    execution_name=f"protein-design-{datetime.now().strftime('%Y%m%d-%H%M%S')}",
    wait=False,
)

print(f"Execution URL: https://flyte.houston.dexdat.ai/console/projects/my-project/domains/production/executions/{execution.id.name}")

# Poll for completion
execution = remote.wait(execution, timeout=timedelta(hours=6))
print(f"Status: {execution.closure.phase}")
```

---

## CLI Operations

```bash
# Register workflows (from CI or local)
pyflyte register workflows/ \
  --project my-project \
  --domain production \
  --image ${AWS_ACCOUNT_ID}.dkr.ecr.us-east-1.amazonaws.com/my-workflows:v1.2.3 \
  --version v1.2.3

# Run locally (fast iteration, no cluster needed)
pyflyte run --local workflows/my_workflow.py my_workflow \
  --input_param "value"

# Run on cluster
pyflyte run workflows/my_workflow.py my_workflow \
  --project my-project \
  --domain development \
  --input_param "value"

# Check execution status
flytectl get execution \
  --project my-project \
  --domain production \
  my-execution-name \
  --output json | tee evidence.json

# List recent executions
flytectl get execution \
  --project my-project \
  --domain production \
  --limit 10

# Get execution logs
flytectl get execution \
  --project my-project \
  --domain production \
  my-execution-name \
  --details
```

---

## Debugging Failed Executions

```bash
# Check Flyte admin logs
kubectl logs -n flyte \
  $(kubectl get pods -n flyte -l app.kubernetes.io/name=flyteadmin -o name | head -1) \
  | grep -E "ERROR|WARN" | tail -30

# Check propeller (the execution engine)
kubectl logs -n flyte \
  $(kubectl get pods -n flyte -l app.kubernetes.io/name=flytepropeller -o name | head -1) \
  | grep -E "ERROR|WARN" | tail -30

# Check a specific task pod
kubectl get pods -n flyte-worker \
  -l "execution-id=my-execution-name" \
  --output wide

# Describe failing pod (look at Events section)
kubectl describe pod <pod-name> -n flyte-worker

# Common issues:
# - OOMKilled: increase mem limits
# - Evicted: node ran out of resources; check resource requests
# - ImagePullBackOff: image tag doesn't exist in ECR; check image spec version
# - Pending: no nodes available with requested resources; check Karpenter logs
```

---

## Cross-Account S3 Access

Flyte stores inputs/outputs in S3. For cross-account bucket access (e.g. Flyte
worker in `houston` account reading data from another account):

```python
# Reference cross-account S3 data in workflows
from flytekit.types.file import FlyteFile
from flytekit.types.directory import FlyteDirectory

@task(
    requests=Resources(cpu="4", mem="16Gi"),
    # The worker SA must have cross-account S3 read via role assumption
)
def load_training_data(s3_path: str) -> FlyteDirectory:
    """Load training data from S3 (cross-account)."""
    return FlyteDirectory(path=s3_path)

# Pass cross-account path at execution time
# s3://other-account-bucket/data/experiment-123/
```

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|---|---|---|
| `@task` without resource requests | OOMKilled; unpredictable scheduling | Always declare `requests=Resources(...)` |
| `:latest` image tag in ImageSpec | Non-reproducible; breaks caching | Pinned semantic version or SHA |
| No `cache=True` on expensive tasks | Re-runs identical computations | Add `cache=True, cache_version="v1"` |
| Massive monolithic workflow | Hard to debug; whole workflow reruns on failure | Break into cacheable sub-tasks |
| GPU task without `accelerator=` | May land on CPU-only node; fails at runtime | Always specify `accelerator=GPUAccelerator(...)` |
| Storing secrets in workflow inputs | Secrets in Flyte console, logs, S3 metadata | Use K8s Secrets via pod environment or ExternalSecrets |
| `wait=True` in production scripts | Blocking; ties up the submitter process | `wait=False` + poll or use notifications |

---

## axiom:trace

`axiom:trace work_item=devops-skills-01 spec=specs/00-PRD.md plan= prompt=.opencode/skills/flyte-shellops/SKILL.md evidence= doc= ops= commit=`
