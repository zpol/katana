output "aws_region" {
  value = var.aws_region
}

output "aws_account_id" {
  value = data.aws_caller_identity.current.account_id
}

output "ecr_repository_name" {
  value = aws_ecr_repository.demo.name
}

output "ecr_repository_url" {
  value = aws_ecr_repository.demo.repository_url
}

output "ecr_registry_id" {
  value = data.aws_caller_identity.current.account_id
}

output "katana_iam_user" {
  value = aws_iam_user.katana.name
}

output "katana_access_key_id" {
  value     = aws_iam_access_key.katana.id
  sensitive = true
}

output "katana_secret_access_key" {
  value     = aws_iam_access_key.katana.secret
  sensitive = true
}

output "eks_cluster_name" {
  value = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  value = module.eks.cluster_endpoint
}

output "eks_cluster_arn" {
  value = module.eks.cluster_arn
}

output "katana_irsa_role_arn" {
  value = aws_iam_role.katana_irsa.arn
}

data "aws_caller_identity" "current" {}
