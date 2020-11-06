disable_mlock = 1
ui = 1

storage "consul" {
    address = "consul:8500"
    path    = "vault"
}

listener "tcp" {
    address = "0.0.0.0:8282"
    tls_disable = 1
}

