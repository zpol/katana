# Ephemeral AWS E2E for KATANA (ECR + Inspector + small EKS)

Disposable stack for end-to-end validation. **Always destroy when finished** to avoid EKS (~$0.10/h) + NAT + node charges.

## What it creates

| Resource | Notes |
|----------|--------|
| ECR repo | `scan_on_push`, force_delete, lifecycle keep 5 |
| Enhanced scanning rule | Filtered to this repo only |
| IAM user + access key | Read-only scanner (`tmp/good.txt`) |
| VPC + NAT (single) | Private subnets for nodes |
| EKS 1.31 | **1× t3.medium** managed node |
| IRSA role | Bound to `katana-system/katana` SA |

State: `tmp/e2e-aws/terraform.tfstate` (gitignored).

## Bootstrap creds

`tmp/c.txt` (admin-ish, gitignored):

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
```

## Apply → deploy → destroy

```bash
bash deploy/e2e-aws/apply.sh          # ~15–25 min (EKS)
bash deploy/e2e-aws/deploy-katana.sh  # build/push image, deploy, smoke tests
bash deploy/e2e-aws/destroy.sh        # tear down ALL (EKS+ECR+IAM+VPC)
```

## Notes

- Katana uses **IRSA** on EKS (not the static keys) for ECR scan reads
- Static keys in `good.txt` remain available for local `go run` testing
- Katana does not start scans; `deploy-katana.sh` pushes a sample image and waits for findings
