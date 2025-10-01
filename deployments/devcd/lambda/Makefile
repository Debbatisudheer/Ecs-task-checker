ACCOUNT = $(shell grep "^account" terraform.tfvars | cut -d '=' -f2 | tr -d ' ' | tr -d '"')
ENV = $(shell grep "^environment" terraform.tfvars | cut -d '=' -f2 | tr -d ' ' | tr -d '"')
SERVICE = $(shell grep "^service" terraform.tfvars | cut -d '=' -f2 | tr -d ' ' | tr -d '"')
# DEPLOYMENTS = $(shell grep "^deployments" terraform.tfvars | cut -d '=' -f2 | tr -d ' ' | tr -d '"')

ifeq ($(USR_SCRIPTS_PATH),)
USR_SCRIPTS_PATH := ../../../scripts
endif

ECSMONITOR_ARTIFACT_TAG = $(shell python3 ${USR_SCRIPTS_PATH}/readOverridesArtifact.py -k ecsmonitorArtifactTag -d overrides.tf.json)
ecsmonitorZipFileName = $(shell grep "^ecsmonitorZip" terraform.tfvars | cut -d '=' -f2 | tr -d ' ' | tr -d '"')

GLOBAL_STATE_BUCKET = $(ACCOUNT)-state
TFSTATE_FILE = $(ENV)/$(SERVICE)/tools/ecsmonitor/terraform.state

ifeq ($(TERRAFORM_BIN_PATH),)
TERRAFORM_BIN_PATH := terraform
endif

.PHONY: check-prereq log-setup setup config plan deploy jenkins output destroy updateipsets

# --- Pre-req check ---
check-prereq:
	@echo "check pre-requisites ..."
	command -v $(TERRAFORM_BIN_PATH) >/dev/null 2>&1 || { echo "terraform is not in PATH. Aborting." >&2; exit 1; }

# --- Setup ---
setup: check-prereq
	@echo "Get the $(ECSMONITOR_ARTIFACT_TAG) artifact Zip files ..."
	aws s3 cp --no-progress s3://sse-serverless-artifacts/ecsmonitor/artifacts/$(ECSMONITOR_ARTIFACT_TAG) ./${ecsmonitorZipFileName}

updateipsets:
	python3 createIPSets.py

# --- Terraform config ---
config:
	rm -rf .terraform
	@echo "Terraform remote S3 backend config for terraform state ..."
	$(TERRAFORM_BIN_PATH) init -backend=true \
		-backend-config="bucket=$(GLOBAL_STATE_BUCKET)" \
		-backend-config="key=$(TFSTATE_FILE)" \
		-backend-config="encrypt=1"

# --- Terraform plan ---
plan: config
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform plan started ..."
	@echo "Terraform get, plan ..."
	$(TERRAFORM_BIN_PATH) get
	$(TERRAFORM_BIN_PATH) plan
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform plan done ..."

# --- Terraform apply ---
deploy:
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform apply started ..."
	$(TERRAFORM_BIN_PATH) apply -auto-approve
	rm -f .terraform/terraform.tfstate*
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform apply done ..."

jenkins:
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform apply started ..."
	$(TERRAFORM_BIN_PATH) apply -input=false -auto-approve
	rm -f .terraform/terraform.tfstate*
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform apply done ..."

# --- Terraform output ---
output: config
	$(TERRAFORM_BIN_PATH) show
	rm -f .terraform/terraform.tfstate*

# --- Terraform destroy ---
destroy: plan
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform destroy started ..."
	$(TERRAFORM_BIN_PATH) destroy
	rm -rf .terraform
	@echo "$$(date +%Y-%m-%dT%H:%M:%S%z): Terraform destroy done ..."
