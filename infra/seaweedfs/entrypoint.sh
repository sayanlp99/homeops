#!/bin/sh
# Starts a single-node SeaweedFS (master + volume + filer + S3 gateway) with one S3
# identity whose keys come from the environment, so no secrets live in the repo.
set -eu
: "${S3_ACCESS_KEY:?set S3_ACCESS_KEY}"
: "${S3_SECRET_KEY:?set S3_SECRET_KEY}"
mkdir -p /etc/seaweedfs /data
cat > /etc/seaweedfs/s3.json <<JSON
{
  "identities": [
    {
      "name": "homeops",
      "credentials": [{ "accessKey": "${S3_ACCESS_KEY}", "secretKey": "${S3_SECRET_KEY}" }],
      "actions": ["Admin", "Read", "Write", "List", "Tagging"]
    }
  ]
}
JSON
exec weed server -dir=/data -ip=seaweedfs -ip.bind=0.0.0.0 \
  -master.volumeSizeLimitMB=1024 -volume.max=0 \
  -s3 -s3.port=8333 -s3.config=/etc/seaweedfs/s3.json
