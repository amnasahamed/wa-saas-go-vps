# Application Load Balancer + ACM cert + Route53 — Phase 0 stub.
# Phase 1: add ACM cert validation, target groups, listener rules.
# Phase 2: add dashboard ALB on the same VPC, separate target group.

resource "aws_security_group" "alb" {
  name        = "${local.name_prefix}-alb"
  description = "Public HTTP/HTTPS ingress"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "${local.name_prefix}-alb-sg" }
}

resource "aws_lb" "api" {
  name               = "${local.name_prefix}-api"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id

  enable_deletion_protection = local.is_prod

  tags = { Name = "${local.name_prefix}-alb" }
}

# Target groups (one per service) are wired up in Phase 1 once the ECS
# services exist; this file is the placeholder.
