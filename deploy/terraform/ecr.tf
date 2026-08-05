# ECR repositories for the two service images.
# GitHub Actions pushes here; ECS task definitions reference these URIs.

resource "aws_ecr_repository" "api" {
  name                 = "niosengine/${var.env}/api"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = { Name = "${local.name_prefix}-ecr-api" }
}

resource "aws_ecr_repository" "bridge" {
  name                 = "niosengine/${var.env}/bridge"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = { Name = "${local.name_prefix}-ecr-bridge" }
}

# Lifecycle: keep last 30 images per repo
resource "aws_ecr_lifecycle_policy" "api" {
  repository = aws_ecr_repository.api.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last 30 images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 30
      }
      action = { type = "expire" }
    }]
  })
}

resource "aws_ecr_lifecycle_policy" "bridge" {
  repository = aws_ecr_repository.bridge.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep last 30 images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 30
      }
      action = { type = "expire" }
    }]
  })
}
