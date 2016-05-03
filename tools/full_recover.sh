#!/bin/bash
set -e

bkfile=`ls -1 /var/lib/mysql_backup/ | awk '{print $(NF)}' | grep 'tar.gz' | sort -r | head -1`
bkname=$(echo $bkfile | awk -F'.' '{print $1}')
prepare_dir=/var/lib/mysql_backup/$bkname
mkdir $prepare_dir
tar -izxvf /var/lib/mysql_backup/$bkfile -C $prepare_dir
echo "Start to recover data"
innobackupex --user=root --use-memory=256M --apply-log $prepare_dir
innobackupex --copy-back $prepare_dir 2>  "/var/log/baklog/recover_$bkname"

chown -R mysql:mysql /var/lib/mysql
cp $prepare_dir/xtrabackup_binlog_info /var/lib/mysql_backup/incrbk_prepare
rm -rf $prepare_dir
