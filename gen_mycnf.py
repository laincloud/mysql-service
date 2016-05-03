#!/usr/bin/python

import ConfigParser
import os
import sys

# memory in Bytes
GB = 1024 * 1024 * 1024
MB = 1024 * 1024
KB = 1024


def gen_mysql_conf(memory, pool_size):
    config = ConfigParser.ConfigParser(allow_no_value=True)
    config.read("etc/templates/my.cnf.tmpl")
    config.set("mysqld", "server-id", os.environ.get("DEPLOYD_POD_INSTANCE_NO"))

    config.set("mysqld", "query_cache_size", 64 * MB if memory / 64 > 64 * MB else memory / 64)
    sessions_available = memory - pool_size
    config.set("mysqld", "innodb_buffer_pool_size", pool_size)

    # 1000 connections
    config.set("mysqld", "sort_buffer_size", sessions_available / 1024 / 4)
    config.set("mysqld", "join_buffer_size", sessions_available / 1024 / 4)

    with open('/etc/my.cnf', 'wb') as configfile:
        config.write(configfile)


def parse_memory(memory, default_bytes):
    scale = memory[len(memory) - 1:]
    mem_bytes = default_bytes

    value = int(memory[0:len(memory) - 1])
    if scale == "K" or scale == "k":
        mem_bytes = KB * value
    elif scale == "M" or scale == "m":
        mem_bytes = MB * value
    elif scale == "G" or scale == "g":
        mem_bytes = GB * value
    elif scale == "B" or scale == "b":
        mem_bytes = value
    return mem_bytes

if __name__ == "__main__":
    args = sys.argv
    memory = 512 * MB
    pool_size = 128 * MB
    if len(args) > 1:
        try:
            memory = parse_memory(args[1], memory)
            if len(args) > 2:
                pool_size = parse_memory(args[2], pool_size)
        except Exception, ex:
            print ex
    print "Memory size {memory}, innodb_buffer_pool_size {pool_size}".format(
        memory=memory, pool_size=pool_size)
    gen_mysql_conf(memory, pool_size)
