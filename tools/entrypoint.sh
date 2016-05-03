#!/bin/bash
set -e

parent=`dirname $0`
source $parent/../conf/secret.conf

get_option () {
	local section=$1
	local option=$2
	local default=$3
	# my_print_defaults can output duplicates, if an option exists both globally and in
	# a custom config file. We pick the last occurence, which is from the custom config.
	ret=$(my_print_defaults $section | grep '^--'${option}'=' | cut -d= -f2- | tail -n1)
	[ -z $ret ] && ret=$default
	echo $ret
}

# if command starts with an option, prepend mysqld
if [ "${1:0:1}" = '-' ]; then
	set -- mysqld "$@"
fi

if [ "$1" = 'mysqld' ]; then

	DBA_PASSWORD=$dba_passwd
	REPL_PASSWORD=$repl_passwd
	DATADIR="$("$@" --verbose --help 2>/dev/null | awk '$1 == "datadir" { print $2; exit }')"
	SOCKET=$(get_option  mysqld socket "$DATADIR/mysqld.sock")
	PIDFILE=$(get_option mysqld pid-file "/var/run/mysqld/mysqld.pid")
	LOG_BIN_DIR="/var/lib/mysql_log_bin"
	BACKUP_DIR="/var/lib/mysql_backup"
	BAK_LOG_DIR="/var/log/baklog"
	RELAY_LOG_DIR="/var/lib/mysql_relay_log"
	SLOW_LOG_DIR="/var/lib/mysql_slow"
	if [ ! -d "$DATADIR/mysql" ]; then
		if [ -z "$DBA_PASSWORD" -o -z "$REPL_PASSWORD" ]; then
			echo >&2 'error: database is uninitialized and DBA_PASSWORD or REPL_PASSWORD not set'
			echo >&2 '  Did you forget to add -e DBA_PASSWORD= or REPL_PASSWORD=... ?'
			exit 1
		fi

		mkdir -p "$DATADIR"
		mkdir -p "$LOG_BIN_DIR"
		mkdir -p "$BACKUP_DIR"
		mkdir -p "$BAK_LOG_DIR"
		mkdir -p "$RELAY_LOG_DIR"
		mkdir -p "$SLOW_LOG_DIR"
		chown -R mysql:mysql "$DATADIR"
		chown -R mysql:mysql "$LOG_BIN_DIR"
		chown -R mysql:mysql "$BACKUP_DIR"
		chown -R mysql:mysql "$BAK_LOG_DIR"
		chown -R mysql:mysql "$RELAY_LOG_DIR"
		chown -R mysql:mysql "$SLOW_LOG_DIR"

		echo 'Running mysql_install_db'
		mysql_install_db --user=mysql --datadir="$DATADIR" --rpm --keep-my-cnf
		echo 'Finished mysql_install_db'

		mysqld --user=mysql --datadir="$DATADIR" --skip-networking &
		for i in $(seq 30 -1 0); do
			[ -S "$SOCKET" ] && break
			echo 'MySQL init process in progress...'
			sleep 1
		done
		if [ $i = 0 ]; then
			echo >&2 'MySQL init process failed.'
			exit 1
		fi

		# sed is for https://bugs.mysql.com/bug.php?id=20545
		#mysql_tzinfo_to_sql /usr/share/zoneinfo | sed 's/Local time zone must be set--see zic manual page/FCTY/' | mysql --protocol=socket -uroot mysql

		# These statements _must_ be on individual lines, and _must_ end with
		# semicolons (no line breaks or comments are permitted).
		# TODO proper SQL escaping on ALL the things D:

		# We only use root@localhost
		tempSqlFile=$(mktemp /tmp/mysql-first-time.XXXXXX.sql)
		cat > "$tempSqlFile" <<-EOSQL
			-- What's done in this file shouldn't be replicated
			--  or products like mysql-fabric won't work
			SET @@SESSION.SQL_LOG_BIN=0;

			DELETE FROM mysql.user WHERE User<>'root' or Host<>'localhost';
			REVOKE PROXY ON ''@'' FROM 'root'@'localhost';
			FLUSH PRIVILEGES ;
			DROP DATABASE IF EXISTS test ;
		EOSQL

		echo "CREATE USER 'repl'@'%' IDENTIFIED BY '"$REPL_PASSWORD"' ;" >> "$tempSqlFile"
		echo "GRANT PROCESS, REPLICATION SLAVE ON *.* TO 'repl'@'%' ;" >> "$tempSqlFile"
		echo "CREATE USER 'dba'@'%' IDENTIFIED BY '"$DBA_PASSWORD"' ;" >> "$tempSqlFile"
		echo "GRANT RELOAD, PROCESS, SUPER, REPLICATION CLIENT, REPLICATION SLAVE ON *.* TO 'dba'@'%' ;" >> "$tempSqlFile"
		echo "FLUSH PRIVILEGES ;" >> "$tempSqlFile"

		mysql --protocol=socket -uroot < "$tempSqlFile"

		rm -f "$tempSqlFile"
		kill $(cat $PIDFILE)
		for i in $(seq 30 -1 0); do
			[ -f "$PIDFILE" ] || break
			echo 'MySQL init process in progress...'
			sleep 1
		done
		if [ $i = 0 ]; then
			echo >&2 'MySQL hangs during init process.'
			exit 1
		fi
		echo 'MySQL init process done. Ready for start up.'
	fi

	chown -R mysql:mysql "$DATADIR"
fi
exec "$@"
