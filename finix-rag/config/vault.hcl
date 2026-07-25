# FINIX RAG - Vault Server Configuration (Dev Mode)
# For production: use proper TLS certs and auto-unseal

storage "file" {
  path = "/vault/file"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = 1  # Set to 0 in production with valid certs
}

api_addr = "http://127.0.0.1:8200"
ui = true
disable_mlock = true
