# Small ephemeral EKS for KATANA E2E (1× t3.medium). Destroy after tests.

data "aws_availability_zones" "available" {
  state = "available"
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.19"

  name = local.name
  cidr = "10.42.0.0/16"

  azs             = slice(data.aws_availability_zones.available.names, 0, 2)
  private_subnets = ["10.42.1.0/24", "10.42.2.0/24"]
  public_subnets  = ["10.42.101.0/24", "10.42.102.0/24"]

  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true
  enable_dns_support   = true

  public_subnet_tags = {
    "kubernetes.io/role/elb" = 1
  }
  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = 1
  }

  tags = local.tags
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.33"

  cluster_name    = local.name
  cluster_version = var.eks_version

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  cluster_endpoint_public_access           = true
  enable_cluster_creator_admin_permissions = true

  # Cost: single small managed node
  eks_managed_node_groups = {
    default = {
      name           = "default"
      instance_types = [var.eks_node_instance_type]
      capacity_type  = "ON_DEMAND"
      min_size       = 1
      max_size       = 1
      desired_size   = 1
      disk_size      = 40

      labels = {
        workload = "katana-e2e"
      }
    }
  }

  tags = local.tags
}

# IRSA: Katana SA can DescribeImageScanFindings without static keys on the pod.
data "aws_iam_policy_document" "katana_irsa_assume" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [module.eks.oidc_provider_arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${replace(module.eks.oidc_provider, "https://", "")}:sub"
      values   = ["system:serviceaccount:katana-system:katana"]
    }
    condition {
      test     = "StringEquals"
      variable = "${replace(module.eks.oidc_provider, "https://", "")}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "katana_irsa" {
  name               = "${local.name}-katana-irsa"
  assume_role_policy = data.aws_iam_policy_document.katana_irsa_assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy" "katana_irsa" {
  name   = "katana-ecr-inspector-read"
  role   = aws_iam_role.katana_irsa.id
  policy = data.aws_iam_policy_document.katana_ecr_read.json
}
