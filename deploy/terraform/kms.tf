# KMS key for envelope-encrypting tenant secrets + RDS storage + S3 SSE.

resource "aws_kms_key" "tenant_secrets" {
  description             = "Tenant secret encryption for niosengine ${var.env}"
  deletion_window_in_days = local.is_prod ? 30 : 7
  enable_key_rotation     = true
  multi_region            = false

  tags = { Name = "${local.name_prefix}-kms" }
}

resource "aws_kms_alias" "tenant_secrets" {
  name          = "alias/${local.name_prefix}-tenant-secrets"
  target_key_id = aws_kms_key.tenant_secrets.key_id
}
