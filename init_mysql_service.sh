#!/bin/bash

./gen_mycnf.py $1 $2

dataExist=`ls /var/lib/mysql`
bkExist=`ls /var/lib/mysql_backup`
if [ -z "$dataExist" -a -n "$bkExist" ]; then
    echo "Start to backup"
    cd tools && ./full_recover.sh
    cd ..
fi
echo "Start to init..."
exec tools/entrypoint.sh mysqld
