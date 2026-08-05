# RDS Postgres 16 — primary database for tenants, users, messages, leads.

resource "aws_db_subnet_group" "main" {
  name       = local.name_prefix
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = "${local.name_prefix}-db-subnet-group" }
}

resource "aws_security_group" "rds" {
  name        = "${local.name_prefix}-rds"
  description = "Allow ECS tasks to reach Postgres"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "Postgres from VPC"
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = [var.vpc_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${local.name_prefix}-rds-sg" }
}

resource "aws_db_instance" "postgres" {
  identifier        = local.name_prefix
  engine            = "postgres"
  engine_version    = "16.4"
  instance_class    = var.db_instance_class
  allocated_storage = var.db_allocated_storage_gb
  storage_type      = "gp3"
  storage_encrypted = true
  kms_key_id        = aws_kms_key.tenant_secrets.arn

  db_name  = var.db_name
  username = var.db_username
  password = random_password.db.result

  multi_az               = local.is_prod
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  backup_retention_period = local.is_prod ? 14 : 3
  backup_window           = "03:00-04:00"
  maintenance_window      = "Mon:00:00-Mon:03:00"

  deletion_protection = local.is_prod
  skip_final_snapshot = !local.is_prod

  performance_insights_enabled          = true
  performance_insights_kms_key_id       = aws_kms_key.tenant_secrets.arn
  performance_insights_retention_period = local.is_prod ? 14 : 7

  tags = { Name = "${local.name_prefix}-postgres" }
}

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "aws_ssm_parameter" "db_url" {
  name        = "/${local.name_prefix}/database-url"
  description = "Postgres connection URL"
  type        = "SecureString"
  value = format(
    "postgres://%s:%s@%s:%d/%s?sslmode=require",
    var.db_username,
    random_password.db.result,
    aws_db_instance.postgres.address,
    aws_db_instance.postgres.port,
    var.db_name,
  )
  tags = { Name = "${local.name_prefix}-db-url" }
}
