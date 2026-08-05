# Terraform — AWS infrastructure for niosengine

## What this provisions

| Resource                | Purpose                                          |
|-------------------------|--------------------------------------------------|
| VPC + public/private subnets + NAT | Network foundation (2 AZs)               |
| ECS Fargate cluster     | Runs `cmd/api` and `cmd/bridge` services        |
| RDS Postgres 16         | Primary database (encrypted at rest)             |
| ElastiCache Redis       | Rate limits, dedupe, reply delay queue           |
| S3 media bucket         | Inbound + outbound WhatsApp media (14-day TTL)   |
| KMS key                 | Envelope encryption for tenant secrets           |
| ECR repos               | Container images for api + bridge               |
| Secrets Manager         | OpenAI + Stripe platform keys                    |
| CloudWatch log groups   | `/ecs/<env>/{api,bridge}`                        |
| ALB                     | Public HTTP/HTTPS ingress (target groups in P1)  |

## Layout

```
terraform/
├── main.tf           provider + locals
├── variables.tf      all input variables
├── outputs.tf        what to surface to CI / dashboard
├── vpc.tf            networking
├── rds.tf            Postgres
├── elasticache.tf    Redis
├── s3.tf             media bucket
├── kms.tf            envelope encryption
├── ecr.tf            container registries
├── alb.tf            load balancer (Phase 0 placeholder)
├── ecs.tf            cluster + services + task defs
├── secrets.tf        Secrets Manager entries
└── terraform.tfvars.example
```

## Bootstrap

```bash
cd deploy/terraform

# 1. Create the S3 backend bucket (one-time, outside this config)
aws s3api create-bucket --bucket niosengine-tfstate-staging --region us-east-1
aws s3api put-bucket-versioning --bucket niosengine-tfstate-staging \
  --versioning-configuration Status=Enabled

# 2. Create a DynamoDB table for state locking
aws dynamodb create-table --table-name niosengine-tfstate-lock \
  --attribute-definitions AttributeName=LockID,AttributeType=S \
  --key-schema AttributeName=LockID,KeyType=HASH \
  --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5

# 3. Uncomment the `backend "s3"` block in main.tf

# 4. Init + apply
terraform init
cp terraform.tfvars.example terraform.tfvars  # edit it
terraform plan -out tfplan
terraform apply tfplan
```

## Per-env workflow

We use Terraform workspaces for staging/prod:

```bash
terraform workspace new staging
terraform workspace select staging
terraform apply -var-file=staging.tfvars

terraform workspace select prod
terraform apply -var-file=prod.tfvars
```

## Image deployment

GitHub Actions (`.github/workflows/deploy.yml`) builds images, pushes to ECR,
then triggers a rolling ECS deploy. The container_image_* vars get injected
at apply time:

```bash
terraform apply \
  -var="container_image_api=$(terraform output -raw ecr_api_repository_url):$GITHUB_SHA" \
  -var="container_image_bridge=$(terraform output -raw ecr_bridge_repository_url):$GITHUB_SHA"
```
