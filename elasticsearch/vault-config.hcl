disable_mlock = 1
ui = 1

storage "file" {
    path = "/mnt/vault"
}

listener "tcp" {
    address = "0.0.0.0:8282"
    tls_disable = 1
}

