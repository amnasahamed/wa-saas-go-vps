# ElastiCache Redis — rate limits, dedupe locks, reply delay queue.

resource "aws_elasticache_subnet_group" "main" {
  name       = local.name_prefix
  subnet_ids = aws_subnet.private[*].id
}

resource "aws_security_group" "redis" {
  name        = "${local.name_prefix}-redis"
  description = "Allow ECS tasks to reach Redis"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "Redis from VPC"
    from_port   = 6379
    to_port     = 6379
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${local.name_prefix}-redis-sg" }
}

resource "aws_elasticache_cluster" "redis" {
  cluster_id           = local.name_prefix
  engine               = "redis"
  engine_version       = "7.1"
  node_type            = var.redis_node_type
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  port                 = 6379

  subnet_group_name    = aws_elasticache_subnet_group.main.name
  security_group_ids   = [aws_security_group.redis.id]
  transit_encryption_enabled = false  # internal-only; switch on for prod
  at_rest_encryption_enabled  = true

  tags = { Name = "${local.name_prefix}-redis" }
}
