#!/usr/bin/python
import requests
import os
import sys


def get_endpoint(method, extra=""):
    return "http://backupctl.{lain_domain}/api/v1/backup/{method}/app/{lain_appname}/proc/{deployd_pod_name}/{extra}".format(
        lain_domain=os.environ.get("LAIN_DOMAIN"),
        lain_appname=os.environ.get("LAIN_APPNAME"),
        deployd_pod_name=os.environ.get("DEPLOYD_POD_NAME"),
        method=method,
        extra=extra)

# Download the latest full backup file
if __name__ == "__main__":
    args = sys.argv
    if len(args) == 1 or (args[1] != "full" and args[1] != "increment"):
        print "Must add the mode 'increment' or 'full'"
    elif args[1] == "full":
        result = requests.request("GET", get_endpoint(method="json"), headers=None, params={"volume": "/var/lib/mysql_backup"}).json()
        result.sort(key=lambda x: x["created"], reverse=True)
        if len(result) > 0:
            print "Find the lastest full backup file: " + result[0]["name"]
            output = requests.post(get_endpoint(method="migrate", extra="file/" + result[0]["name"]), data={
                "volume": result[0]["volume"],
                "from": result[0]["instanceNo"],
                "to": os.environ.get("DEPLOYD_POD_INSTANCE_NO")
            }, allow_redirects=False)
            print output
        else:
            print "No full backup files found"
    else:
        result = requests.request("GET", get_endpoint(method="json"), headers=None, params={"volume": "/var/lib/mysql_log_bin"}).json()
        result.sort(key=lambda x: x["created"], reverse=True)
        if len(result) > 0:
            print "Find the lastest increment backup file: " + result[0]["name"]
            output = requests.post(get_endpoint(method="migrate/increment", extra="dir/" + result[0]["name"]), data={
                "volume": "/var/lib/mysql_backup",
                "from": result[0]["instanceNo"],
                "to": os.environ.get("DEPLOYD_POD_INSTANCE_NO"),
                "files": "*"
            }, allow_redirects=False)
            print output
        else:
            print "No increment backup files found"
