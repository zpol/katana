# Ephemeral AWS stack for KATANA ECR / Inspector E2E.
# Apply with bootstrap creds, use outputs (and tmp/good.txt) for Katana, then:
#   terraform destroy
# to avoid ongoing cost.

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

resource "random_id" "suffix" {
  byte_length = 3
}

locals {
  name = "${var.name_prefix}-${random_id.suffix.hex}"
  tags = merge(var.tags, {
    Project   = "katana"
    Purpose   = "e2e"
    ManagedBy = "terraform"
    Name      = local.name
  })
}

# —— ECR ——
resource "aws_ecr_repository" "demo" {
  name                 = local.name
  image_tag_mutability = "MUTABLE"
  force_delete         = true

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = local.tags
}

resource "aws_ecr_lifecycle_policy" "demo" {
  repository = aws_ecr_repository.demo.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last 5 images (cost control)"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 5
      }
      action = { type = "expire" }
    }]
  })
}

# Registry-wide enhanced scanning (Amazon Inspector). Destroy leaves registry config;
# we set continuous filters only for this repo to limit Inspector cost.
resource "aws_ecr_registry_scanning_configuration" "enhanced" {
  scan_type = "ENHANCED"

  rule {
    scan_frequency = "SCAN_ON_PUSH"
    repository_filter {
      filter      = aws_ecr_repository.demo.name
      filter_type = "WILDCARD"
    }
  }
}

# —— IAM user for Katana (scoped; destroy removes keys) ——
resource "aws_iam_user" "katana" {
  name = "${local.name}-scanner"
  path = "/katana-e2e/"
  tags = local.tags
}

resource "aws_iam_access_key" "katana" {
  user = aws_iam_user.katana.name
}

data "aws_iam_policy_document" "katana_ecr_read" {
  statement {
    sid    = "ECRAuth"
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken",
    ]
    resources = ["*"]
  }

  statement {
    sid    = "ECRReadScan"
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:DescribeImageScanFindings",
      "ecr:DescribeRepositories",
      "ecr:GetDownloadUrlForLayer",
      "ecr:GetRepositoryPolicy",
      "ecr:ListImages",
    ]
    resources = [aws_ecr_repository.demo.arn]
  }

  statement {
    sid    = "InspectorRead"
    effect = "Allow"
    actions = [
      "inspector2:ListFindings",
      "inspector2:BatchGetAccountStatus",
      "inspector2:ListCoverage",
    ]
    resources = ["*"]
  }
}

resource "aws_iam_user_policy" "katana" {
  name   = "katana-ecr-inspector-read"
  user   = aws_iam_user.katana.name
  policy = data.aws_iam_policy_document.katana_ecr_read.json
}

# Optional: allow the bootstrap principal to push a seed image via same user (E2E push helper).
# Separate push user keeps scanner keys read-only — push uses bootstrap creds in scripts.
