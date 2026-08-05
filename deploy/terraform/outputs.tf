output "vpc_id" {
  value       = module.vpc.vpc_id
  description = "VPC ID"
}

output "api_alb_dns_name" {
  value       = aws_lb.api.dns_name
  description = "Public DNS of the cmd/api ALB"
}

output "api_service_name" {
  value       = aws_ecs_service.api.name
  description = "ECS service name for cmd/api"
}

output "bridge_service_name" {
  value       = aws_ecs_service.bridge.name
  description = "ECS service name for cmd/bridge"
}

output "rds_endpoint" {
  value       = aws_db_instance.postgres.endpoint
  description = "RDS Postgres endpoint (host:port)"
  sensitive   = true
}

output "redis_endpoint" {
  value       = aws_elasticache_cluster.redis.cache_nodes[0].address
  description = "ElastiCache Redis endpoint"
  sensitive   = true
}

output "media_bucket" {
  value       = aws_s3_bucket.media.bucket
  description = "S3 bucket for inbound/outbound media"
}

output "kms_key_id" {
  value       = aws_kms_key.tenant_secrets.arn
  description = "KMS key ARN for tenant-secret envelope encryption"
}

output "ecr_api_repository_url" {
  value       = aws_ecr_repository.api.repository_url
  description = "ECR repository URL for cmd/api"
}

output "ecr_bridge_repository_url" {
  value       = aws_ecr_repository.bridge.repository_url
  description = "ECR repository URL for cmd/bridge"
}
