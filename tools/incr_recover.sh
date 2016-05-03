#!/bin/bash
set -e
parent=`dirname $0`
bkdir=/var/lib/mysql_backup
lb_name=$(head -1 $bkdir/incrbk_prepare | awk '{print $1}')
lb_pos=$(head -1 $bkdir/incrbk_prepare | awk '{print $2}')

echo "WARNING: Please execute 'RESET MASTER; SET GLOBAL gtid_purged=xxx' first where xxx is the Executed_Gtid_Set in $bkdir/incrbk_prepare !"

for log in $(ls -1 $bkdir/ | egrep 'lb.[0-9]*$' | sort); do
    if [ "$log" == "$lb_name" ]; then
        echo "Start to restore data from binlog $log"
        mysqlbinlog --start-position=$lb_pos $bkdir/$log | mysql -uroot
    elif [[ "$log" > "$lb_name" ]]; then
        echo "Restore data from binlog $log"
        mysqlbinlog $bkdir/$log | mysql -uroot
    else
        echo "Skip $log"
    fi
    rm -f $bkdir/$log
done
