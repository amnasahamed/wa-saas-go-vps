terraform {
  required_version = ">= 1.9"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }

  # S3 backend is configured per-env in the staging/prod subdirs.
  # Locally, comment out the backend block to use local state.
  backend "s3" {
    # bucket = "niosengine-tfstate-${var.env}"
    # key    = "terraform.tfstate"
    # region = "us-east-1"
    # dynamodb_table = "niosengine-tfstate-lock"
    # encrypt        = true
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "niosengine"
      Environment = var.env
      ManagedBy   = "terraform"
    }
  }
}

locals {
  name_prefix = "niosengine-${var.env}"
  is_prod     = var.env == "prod"
}
