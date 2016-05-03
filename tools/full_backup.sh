#!/bin/bash
set -e
url=$(echo $LAIN_APPNAME | awk -F '.' '{if(NF == 1) print $1; else print $3"."$2"."$1}')
role=$(curl "http://$url.$LAIN_DOMAIN/role?host=$LAIN_PROCNAME-$DEPLOYD_POD_INSTANCE_NO&port=3306")
if [ "$role" != "Master" ]; then
    exit 1
fi
backup_time=`date +'%Y-%m-%d-%H-%M-%S'` #以备份的时间点为备份文件名
#开始备份，并将备份程序输出保存到 /var/log/baklog/ 下面
innobackupex --slave-info --user=root --stream=tar ./ 2>  "/var/log/baklog/$backup_time" | gzip - > "/var/lib/mysql_backup/$backup_time"_bak.tar.gz
result=`tail -1 /var/log/baklog/$backup_time | awk '{print $(NF-1)" "$(NF)}'`
if [ "$result" != "completed OK!" ]; then
    exit 1
fi
rm -rf /var/log/baklog/$backup_time
