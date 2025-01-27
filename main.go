package main

import (
	log "github.com/sirupsen/logrus"
)

func main() {
	flags := getCLflags()
	conf := NewConfig()

	switch flags.action {
	case "es_backup":
		elastic, err := newElasticClient(conf.elastic)
		if err != nil {
			log.Fatal(err)
		}
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := elastic.backupDocuments(sb, conf.publicKeyPath, flags.name); err != nil {
			log.Fatal(err)
		}
	case "es_restore":
		elastic, err := newElasticClient(conf.elastic)
		if err != nil {
			log.Fatal(err)
		}
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := elastic.restoreDocuments(sb, conf.privateKeyPath, flags.name, conf.c4ghPassword); err != nil {
			log.Fatal(err)
		}
	case "mongo_dump":
		mongo := conf.mongo
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := mongo.dump(*sb, conf.publicKeyPath, flags.name); err != nil {
			log.Fatal(err)
		}
	case "mongo_restore":
		mongo := conf.mongo
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := mongo.restore(*sb, conf.privateKeyPath, flags.name, conf.c4ghPassword); err != nil {
			log.Fatal(err)
		}
	case "pg_dump":
		pg := conf.db
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := pg.dump(*sb, conf.publicKeyPath); err != nil {
			log.Fatal(err)
		}
	case "pg_restore":
		pg := conf.db
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := pg.restore(*sb, conf.privateKeyPath, flags.name, conf.c4ghPassword); err != nil {
			log.Fatal(err)
		}
	case "pg_basebackup":
		pg := conf.db
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := pg.basebackup(*sb, conf.publicKeyPath); err != nil {
			log.Fatal(err)
		}
	case "pg_db-unpack":
		pg := conf.db
		sb, err := newS3Backend(conf.s3)
		if err != nil {
			log.Fatal("Could not connect to s3 backend: ", err)
		}

		if err := pg.baseBackupUnpack(*sb, conf.privateKeyPath, flags.name, conf.c4ghPassword); err != nil {
			log.Fatal(err)
		}
	case "backup_bucket":
		src, err := newS3Backend(conf.s3Source)
		if err != nil {
			log.Fatal("Could not connect to s3 source backend: ", err)
		}

		dst, err := newS3Backend(conf.s3Destination)
		if err != nil {
			log.Fatal("Could not connect to s3 destnation backend: ", err)
		}

		if err = BackupS3BuckeEncrypted(src, dst, conf.publicKeyPath); err != nil {
			log.Fatal(err)
		}
	case "restore_bucket":
		src, err := newS3Backend(conf.s3Source)
		if err != nil {
			log.Fatal("Could not connect to s3 source backend: ", err)
		}

		dst, err := newS3Backend(conf.s3Destination)
		if err != nil {
			log.Fatal("Could not connect to s3 destnation backend: ", err)

		}
		err = RestoreEncryptedS3Bucket(src, dst, conf.c4ghPassword, conf.privateKeyPath)
		if err != nil {
			log.Fatal(err)
		}
	case "sync_buckets":
		src, err := newS3Backend(conf.s3Source)
		if err != nil {
			log.Fatal("Could not connect to s3 source backend: ", err)
		}

		dst, err := newS3Backend(conf.s3Destination)
		if err != nil {
			log.Fatal("Could not connect to s3 destnation backend: ", err)

		}
		err = SyncS3Buckets(src, dst)
		if err != nil {
			log.Fatal(err)
		}
	}
}
