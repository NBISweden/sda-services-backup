# Configure config file 

resource "kubernetes_secret_v1" "Your_secret_name" {
  metadata {
    name      = "backup-secret"
    namespace = "namespace"
  }
  data = {
    "key.pub.pem" =  "Crypt4gh public key from vault",
    "key.sec.pem," = "Crypt4gh private key from vault",
    "config.yaml" = yamlencode({
      # configuring path to where keys will be mounted
      "crypt4ghPublicKey" : "/.secrets/keys/key.pub.pem",
      "crypt4ghPrivateKey" :"/.secrets/keys/key.sec.pem",
      "crypt4ghPassphrase" : "your passphrase"
      "loglevel": "debug",
      "db" : {
        "host" : "IP address of DB or name",
        "user" : "postgres",
        "password" : "password",
        "database" : "postgres",
        "sslmode" : "disable",
        "cacert": "path/to/ca-root",
        "clientcert": "path/to/clientcert", #only needed if sslmode = verify-peer
        "clientkey": "path/to/clientkey" #only needed if sslmode = verify-peer
        "sslmode": "verify-peer" 
      },
      "s3" : {
        "url": "FQDN URI" #https://s3.example.com
        #"port": "9000" #only needed if the port difers from the standard HTTP/HTTPS ports
       "accesskey": "accesskey"
       "secretkey": "secret-accesskey"
       "bucket": "bucket-name"
       #"cacert": "path/to/ca-root"
      },
      "elastic":{
      "host": "FQDN URI" # https://es.example.com
      #port: 9200 # only needed if the port difers from the standard HTTP/HTTPS ports
      "user": "elastic-user"
      "password": "elastic-password"
      #cacert: "path/to/ca-root"
      "batchSize": "50" # How many documents to retrieve from elastic search at a time, default 50 (should probably be at least 2000
      "filePrefix": "" # Can be emtpy string, useful in case an index has been written to and you want to backup a new copy
      },
      "mongo":{
       "host": "hostname or IP with portnuber" #example.com:portnumber, 127.0.0.1:27017
      "user": "backup"
      "password": "backup"
      "authSource": "admin"
      "replicaset": ""
     #"tls": "true"
     #"cacert": "path/to/ca-root" #optional
     #"clientcert": "path/to/clientcert" # needed if tls=true
      }
    })
  }
}



resource "kubernetes_cron_job" "Your_cronjob_name" {
  metadata {
    name      = "Your_cronjob_name"
    namespace = "Your_namespace_name"
  }
  spec {
    concurrency_policy        = "Forbid"
    schedule                  = "0 0 1 * *"
    starting_deadline_seconds = 3600
    job_template {
      metadata {
        name = "Your_cronjob_name"
      }
      spec {
        backoff_limit              = 2
        ttl_seconds_after_finished = 120
        template {
          metadata {
            labels = {
              jobs = "backup"
            }
          }
          spec {
            container {
              name    = "Container_name"
              image   = "ghcr.io/nbisweden/sda-services-backup"
              command = ["/usr/local/bin/backup-svc", "--action", "pg_dump"]
            env {
                name  = "CONFIGFILE"
                value = "/.secrets/config.yaml"
            }
            resources {
                limits = {
                  cpu    = "100m"
                  memory = "128M"
                }
                requests = {
                  cpu    = "100m"
                  memory = "128M"
                }
            }
            security_context {
                allow_privilege_escalation = "false"
                read_only_root_filesystem  = "true"
                capabilities {
                  drop = [ "ALL" ]
                }
              }
              volume_mount {
                name       = "tmp"
                mount_path = "/tmp"
              }
              volume_mount {
                name       = "tls-certs"
                mount_path = "/.secrets/tls"
              }
              # Mounting config file declared above
              volume_mount {
                name       = "name"
                mount_path = "/.secrets/config.yaml"
                sub_path   = "config.yaml"
              }
              # Crytp4gh public key
            volume_mount {
                name       = "name"
                mount_path = "/.secrets/keys/key.pub.pem"
                sub_path   = "key.pub.pem"
              }
              # Crytp4gh private key
            volume_mount {
                name       = "name"
                mount_path = "/.secrets/keys/key.sec.pem"
                sub_path   = "key.sec.pem"
            }
            }
            security_context {
              fs_group = 1000
              run_as_user = 1000
              run_as_group =  1000
              seccomp_profile {
                type = "RuntimeDefault"
              }
            }
            volume {
              name = "tmp"
              empty_dir {
                size_limit = "1Gi"
              }
            }
            volume {
              name = "cronjob"
              projected {
                default_mode = "0400"
                sources {
                  secret {
                    name = kubernetes_secret_v1.Your_secret_name.metadata[0].name
                  }
                }
              }
            }
            volume {
              name = "tls-certs"
              projected {
                default_mode = "0400"
                sources {
                  secret {
                    name = "your_certs"
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}