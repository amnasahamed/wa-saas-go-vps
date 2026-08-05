# S3 bucket for inbound + outbound media. Public-read is disabled;
# dashboard fetches via presigned URLs.

resource "aws_s3_bucket" "media" {
  bucket = "${local.name_prefix}-media"
  tags   = { Name = "${local.name_prefix}-media" }
}

resource "aws_s3_bucket_public_access_block" "media" {
  bucket = aws_s3_bucket.media.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "media" {
  bucket = aws_s3_bucket.media.id
  versioning_configuration { status = "Enabled" }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "media" {
  bucket = aws_s3_bucket.media.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.tenant_secrets.arn
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "media" {
  bucket = aws_s3_bucket.media.id

  rule {
    id     = "expire-temp"
    status = "Enabled"
    filter { prefix = "tenants/*/inbound/" }

    expiration {
      days = 14   # matches legacy MEDIA_TTL_DAYS default
    }
  }
}
