# ECS Fargate cluster + two services (api, bridge).
# Phase 0: cluster + IAM roles + log groups + minimal task defs.
# Phase 1: real container definitions, target group attachments, autoscaling.

resource "aws_ecs_cluster" "main" {
  name = local.name_prefix

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = { Name = "${local.name_prefix}-ecs" }
}

# --- IAM roles ---

data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "task_execution" {
  name               = "${local.name_prefix}-ecs-exec"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = { Name = "${local.name_prefix}-ecs-exec" }
}

resource "aws_iam_role_policy_attachment" "task_execution" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Allow pulling secrets from SSM Parameter Store at task start
resource "aws_iam_role_policy" "task_execution_ssm" {
  name = "ssm-secrets"
  role = aws_iam_role.task_execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "ssm:GetParameters",
        "secretsmanager:GetSecretValue",
        "kms:Decrypt",
      ]
      Resource = ["*"]
    }]
  })
}

resource "aws_iam_role" "task" {
  name               = "${local.name_prefix}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = { Name = "${local.name_prefix}-ecs-task" }
}

resource "aws_iam_role_policy" "task_runtime" {
  name = "runtime"
  role = aws_iam_role.task.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:ListBucket",
        ]
        Resource = [
          aws_s3_bucket.media.arn,
          "${aws_s3_bucket.media.arn}/*",
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Encrypt",
          "kms:Decrypt",
          "kms:GenerateDataKey",
        ]
        Resource = [aws_kms_key.tenant_secrets.arn]
      },
      {
        Effect = "Allow"
        Action = [
          "ses:SendEmail",
          "ses:SendTemplatedEmail",
        ]
        Resource = ["*"]
      },
    ]
  })
}

# --- CloudWatch log groups ---

resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${local.name_prefix}/api"
  retention_in_days = local.is_prod ? 30 : 7
  tags              = { Name = "${local.name_prefix}-log-api" }
}

resource "aws_cloudwatch_log_group" "bridge" {
  name              = "/ecs/${local.name_prefix}/bridge"
  retention_in_days = local.is_prod ? 30 : 7
  tags              = { Name = "${local.name_prefix}-log-bridge" }
}

# --- Task definitions (Phase 0 stub) ---

resource "aws_ecs_task_definition" "api" {
  family                   = "${local.name_prefix}-api"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.api_cpu
  memory                   = var.api_memory_mb
  execution_role_arn       = aws_iam_role.task_execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([
    {
      name      = "api"
      image     = var.container_image_api
      essential = true
      portMappings = [{
        containerPort = 8080
        protocol      = "tcp"
      }]
      environment = [
        { name = "SERVICE_NAME", value = "api" },
        { name = "ENV", value = var.env },
        { name = "API_LISTEN_ADDR", value = ":8080" },
      ]
      secrets = [
        { name = "DATABASE_URL", valueFrom = aws_ssm_parameter.db_url.arn },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.api.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "api"
        }
      }
    }
  ])

  tags = { Name = "${local.name_prefix}-task-api" }
}

resource "aws_ecs_task_definition" "bridge" {
  family                   = "${local.name_prefix}-bridge"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = var.bridge_cpu
  memory                   = var.bridge_memory_mb
  execution_role_arn       = aws_iam_role.task_execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([
    {
      name      = "bridge"
      image     = var.container_image_bridge
      essential = true
      portMappings = [{
        containerPort = 9090
        protocol      = "tcp"
      }]
      environment = [
        { name = "SERVICE_NAME", value = "bridge" },
        { name = "ENV", value = var.env },
        { name = "BRIDGE_GRPC_ADDR", value = ":9090" },
      ]
      secrets = [
        { name = "DATABASE_URL", valueFrom = aws_ssm_parameter.db_url.arn },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.bridge.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "bridge"
        }
      }
    }
  ])

  tags = { Name = "${local.name_prefix}-task-bridge" }
}

# --- Services ---

resource "aws_security_group" "ecs_tasks" {
  name        = "${local.name_prefix}-ecs-tasks"
  description = "Allow ECS tasks to reach RDS, Redis, ALB"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "From VPC"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${local.name_prefix}-ecs-tasks-sg" }
}

resource "aws_ecs_service" "api" {
  name            = "${local.name_prefix}-api"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.api_desired_count
  launch_type     = "FARGATE"

  network {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = false
  }

  # target group attachment added in Phase 1

  tags = { Name = "${local.name_prefix}-svc-api" }
}

resource "aws_ecs_service" "bridge" {
  name            = "${local.name_prefix}-bridge"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.bridge.arn
  desired_count   = var.bridge_desired_count
  launch_type     = "FARGATE"

  network {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = false
  }

  tags = { Name = "${local.name_prefix}-svc-bridge" }
}
