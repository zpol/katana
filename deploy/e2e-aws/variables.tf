variable "aws_region" {
  type        = string
  description = "AWS region for ECR / Inspector / EKS"
  default     = "eu-west-1"
}

variable "name_prefix" {
  type        = string
  description = "Prefix for ephemeral resource names"
  default     = "katana-e2e"
}

variable "eks_version" {
  type        = string
  description = "EKS Kubernetes version"
  default     = "1.31"
}

variable "eks_node_instance_type" {
  type        = string
  description = "Single managed-node instance type"
  default     = "t3.medium"
}

variable "tags" {
  type        = map(string)
  description = "Extra tags"
  default     = {}
}
