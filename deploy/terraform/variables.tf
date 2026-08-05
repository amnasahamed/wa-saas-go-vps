variable "env" {
  description = "Environment name (dev, staging, prod)"
  type        = string
  default     = "staging"
}

variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "domain_name" {
  description = "Primary domain (e.g. niosengine.app). Used for ALB + ACM certs."
  type        = string
  default     = ""
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t4g.small"
}

variable "db_allocated_storage_gb" {
  description = "RDS allocated storage in GB"
  type        = number
  default     = 20
}

variable "db_name" {
  description = "Postgres database name"
  type        = string
  default     = "niosengine"
}

variable "db_username" {
  description = "Postgres master username"
  type        = string
  default     = "niosengine"
}

variable "redis_node_type" {
  description = "ElastiCache Redis node type"
  type        = string
  default     = "cache.t4g.micro"
}

variable "api_desired_count" {
  description = "Desired count of cmd/api ECS tasks"
  type        = number
  default     = 2
}

variable "bridge_desired_count" {
  description = "Desired count of cmd/bridge ECS tasks"
  type        = number
  default     = 2
}

variable "api_cpu" {
  description = "Fargate CPU units for cmd/api task"
  type        = number
  default     = 1024
}

variable "api_memory_mb" {
  description = "Fargate memory (MiB) for cmd/api task"
  type        = number
  default     = 2048
}

variable "bridge_cpu" {
  description = "Fargate CPU units for cmd/bridge task"
  type        = number
  default     = 2048
}

variable "bridge_memory_mb" {
  description = "Fargate memory (MiB) for cmd/bridge task"
  type        = number
  default     = 4096
}

variable "container_image_api" {
  description = "ECR image URI for cmd/api (set by CI before terraform apply)"
  type        = string
  default     = "niosengine/api:placeholder"
}

variable "container_image_bridge" {
  description = "ECR image URI for cmd/bridge (set by CI before terraform apply)"
  type        = string
  default     = "niosengine/bridge:placeholder"
}

variable "openai_api_key" {
  description = "OpenAI platform key (sensitive; passed via TF_VAR_openai_api_key env or tfvars)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "stripe_secret_key" {
  description = "Stripe secret key (sensitive)"
  type        = string
  sensitive   = true
  default     = ""
}

variable "stripe_webhook_secret" {
  description = "Stripe webhook signing secret (sensitive)"
  type        = string
  sensitive   = true
  default     = ""
}
