# Secrets Manager entries for non-DB secrets (Stripe, OpenAI platform key).
# DB password is in SSM Parameter Store (rds.tf).

resource "aws_secretsmanager_secret" "openai_key" {
  name                    = "${local.name_prefix}/openai-api-key"
  description             = "OpenAI platform API key (sensitive)"
  kms_key_id              = aws_kms_key.tenant_secrets.arn
  recovery_window_in_days = local.is_prod ? 30 : 7
  tags                    = { Name = "${local.name_prefix}-openai" }
}

resource "aws_secretsmanager_secret_version" "openai_key" {
  count         = var.openai_api_key != "" ? 1 : 0
  secret_id     = aws_secretsmanager_secret.openai_key.id
  secret_string = var.openai_api_key
}

resource "aws_secretsmanager_secret" "stripe" {
  name                    = "${local.name_prefix}/stripe"
  description             = "Stripe secret key + webhook signing secret"
  kms_key_id              = aws_kms_key.tenant_secrets.arn
  recovery_window_in_days = local.is_prod ? 30 : 7
  tags                    = { Name = "${local.name_prefix}-stripe" }
}

resource "aws_secretsmanager_secret_version" "stripe" {
  count         = var.stripe_secret_key != "" ? 1 : 0
  secret_id     = aws_secretsmanager_secret.stripe.id
  secret_string = jsonencode({
    secret_key      = var.stripe_secret_key
    webhook_secret  = var.stripe_webhook_secret
  })
}
