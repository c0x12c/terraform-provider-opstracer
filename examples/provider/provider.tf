terraform {
  required_providers {
    opstracer = {
      source  = "c0x12c/opstracer"
      version = "~> 0.2.2"
    }
  }
}

# Or leave both unset and export OPSTRACER_API_URL and OPSTRACER_TOKEN.
provider "opstracer" {
  api_url = "https://api-control-plane.opstracer.ai"
  token   = var.opstracer_token
}

variable "opstracer_token" {
  type      = string
  sensitive = true
}
