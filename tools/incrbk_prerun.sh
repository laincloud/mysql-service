#!/bin/bash
set -e
url=$(echo $LAIN_APPNAME | awk -F '.' '{if(NF == 1) print $1; else print $3"."$2"."$1}')
role=$(curl "http://$url.$LAIN_DOMAIN/role?host=$LAIN_PROCNAME-$DEPLOYD_POD_INSTANCE_NO&port=3306")
if [ "$role" != "Master" ]; then
    exit 1
fi
