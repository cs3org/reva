ui = true
default_lease_ttl = "168h"
max_lease_ttl = "720h"
disable_mlock = true

storage "file" {
  path = "cert/data"
}

listener "tcp" {
  address     = "localhost:8200"
  tls_cert_file = "cert/vault.pem"
  tls_key_file = "cert/vault.key"
}