variable "function_name" {
  type = string
}

/* 
variable "s3_bucket" { type = string }
variable "s3_key" { type = string }
*/

variable "ecs_service_name" {
  type = string
}

variable "region" {
  type = string
}

variable "account" {
  type = string
}

variable "environment" {
  type = string
}

variable "service" {
  type = string
}

variable "aws_source" {
  type = string
}

variable "terraform_current_version" {
  type = string
}

variable "lambda_zip_file" {
  type    = string
  default = "function.zip"
}

variable "aws_provider_version" {
  type = string
}

variable "lambda_s3_key" {
  description = "The S3 object key (filename) for the Lambda zip file"
  type        = string
}

variable "lambda_s3_bucket" {
  type        = string
  description = "The name of the existing S3 bucket for Lambda zip file"
}

variable "lambda_s3_folder" {
  description = "The folder inside the S3 bucket where the Lambda zip file will be stored"
  type        = string
  default     = "ecs_task_checker/artifacts/"
}

variable "log_retention_in_days" {
  description = "Number of days to retain CloudWatch logs"
  type        = number
  default     = 14
}

variable "owner" {
  description = "Owner tag for resources"
  type        = string
}

variable "ecsmonitorZip" {
  description = "Path to the zip file containing the Lambda function code for ecs_monitor"
  type        = string
}


terraform {
  backend "s3" {
    bucket  = "123456789012-state"
    key     = "dev/ecstaskchecker/tools/ecstaskanalyzer/terraform.state"
    region  = "us-east-1"
    encrypt = true
  }
}