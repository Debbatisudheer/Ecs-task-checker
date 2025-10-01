data "aws_iam_role" "existing_role" {
  name = "evt-lambda-role"
}

resource "aws_cloudwatch_log_group" "ecsmonitor" {
  name              = "/aws/lambda/${var.service}-ecsmonitor"
  retention_in_days = var.log_retention_in_days

  tags = {
    Environment = var.environment
    Project     = var.environment
    Service     = var.service
    Owner       = var.owner
  }
}

resource "aws_lambda_function" "ecsmonitor" {
  function_name = "${var.service}-ecsmonitor"
  handler       = "bootstrap"
  runtime       = "provided.al2"
  role          = data.aws_iam_role.existing_role.arn
  timeout       = 10
  source_code_hash = filebase64sha256(var.ecsmonitorZip)
  filename         = var.ecsmonitorZip

  depends_on = [
    aws_cloudwatch_log_group.ecsmonitor
  ]

  environment {
    variables = {
      ENVIRONMENT = var.environment
    }
  }
}

resource "aws_lambda_function_url" "ecsmonitor_url" {
  function_name     = aws_lambda_function.ecsmonitor.function_name
  authorization_type = "NONE"

  cors {
    allow_origins = ["*"]
    allow_methods = ["GET", "POST"]
    allow_headers = ["*"]
  }
}
