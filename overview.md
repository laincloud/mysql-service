### 2.4 开发者相关

#### 2.4.1 备份脚本（tools/full_backup.sh）
直接运行该文件可以在`/var/lib/mysql\_backup`生成备份的压缩文件，文件名`为yyyy_mm_dd_hh_MM_SS_bak.tar.gz`。如果集群配置了backupd，该脚本交给backupd自动调用，以实现自动全量备份。

该脚本运行前会向monitor发送一个GET请求，该请求会返回该节点的角色。如果角色为`Master`，则执行备份操作。否则中断返回1。

运行后，该脚本会检查备份日志，如果日志中出现了备份成功的信息，会正常结束，交给backupd处理。否则将返回1，如果集群配置了Graphite监控系统，则将备份返回值（0或1）发送到Graphite。

backupd将备份文件保存到volume backup后，会清理容器中的备份文件。

#### 2.4.2 全量恢复脚本（tools/full_recover.sh）

   当MySQL数据文件夹`/var/log/mysql`为空**且**`/var/lib/mysql_backup`中有备份后的压缩文件时，可以进行数据恢复。该脚本会在`/var/lib/mysql_backup`中找到最新的压缩文件解压并恢复数据。

#### 2.4.3 容器主脚本（init\_mysql\_service.sh）

该文件是mysql-server proc的启动执行文件。它可以从secret_files中得到配置的root、dba和repl用户的密码信息，并设置相应的环境变量，更新拷贝my.cnf模版。

> 如果集群没有配置**lvault**，则需要初始化前在代码库中的conf/secret.conf加入如下格式的配置，并重新编译部署。
> ```ini
> root_passwd=your_root_password
> repl_passwd=your_repl_password
> dba_passwd=your_dba_password
> client_id=mysql_service_app_client_id
> secret=mysql_service_app_secret
> ```
>

最后根据数据文件夹的情况执行不同的操作：

- 如果`/var/lib/mysql`和`/var/lib/mysql_backup`均为空，则认为该instance是第一次启动，初始化mysqld。
- 如果`/var/lib/mysql`为空，`/var/lib/mysql_backup`不为空，则认为该instance需要数据恢复，执行`tools/recover.sh`脚本。数据恢复后，直接启动mysqld。
- 如果/`var/lib/mysql`不为空，则认为该instance是正常启动，直接启动mysqld。

#### 2.4.4 实例初始化脚本（tools/entrypoint.sh）

该文件修改自官方的mysql:latest。增加了root、repl、dba用户建立与权限配置。

> root、repl、dba三者的权限如下：
> - root用户是超级用户，具有所有权限，该用户是数据全量备份和恢复时使用。
> - repl是主从同步时同步线程的连接用户，具有`PROCESS, REPLICATION SLAVE`权限。
> - dba是运维和监控时的用户，具有`RELOAD, PROCESS, SUPER, REPLICATION CLIENT, REPLICATION SLAVE`权限。

#### 2.4.5 清理脚本（tools/clean.sh）

清理mysql_backup文件夹备份文件的脚本，当每次备份成功后，会执行清理工作，防止旧的备份重复上传到volume backup。并将备份成功的信息发送给监控系统。

#### 2.4.7 tools/download.py

从volume backup中migrate最新的全量备份压缩包或增量备份的所有binlog到当前的container中的/var/lib/mysql_backup。是数据恢复或节点扩容的准备操作。

执行参数如下

- /lain/app/tools/download.py full 下载最新的全量备份压缩包
- /lain/app/tools/download.py increment 下载所有的增量备份的binlog文件

#### 2.4.8 tools/incrbk_prerun.sh

执行增量备份时的前置条件检查脚本，主要是检查自己的角色是否为Standby。如果为Standby则执行增量备份，否则返回1终止增量备份。

#### 2.4.9 tools/incr_recover.sh

增量恢复脚本。当执行完全量恢复后，如果已经下载了所有的增量备份的binlog文件，手动运行该脚本可以继续完成增量恢复。

执行时，该脚本会排序所有的binlog，并跳过全量备份点之前的binlog，从备份点开始恢复之后的所有数据。

每处理完一个binlog文件会将其删除

## 3 Service使用说明

### 3.1 Service引用

MySQL Service提供了*mysql-master*和*mysql-slave*两种service。mysql-master提供对master节点的连接，可读写。mysql-slave提供对slave节点的连接，并可以将请求随机分配到某个slave上，但是只读。

在app的lain.yaml文件中增加如下配置，根据实际需要配置mysql-master或mysql-slave。

```yaml
use_services:
    mysql-service:
        - mysql-master #MySQL连接的host=mysql-master port=3306
        - mysql-slave  #MySQL连接的host=mysql-slave port=3306
```

### 3.2 Service横向扩容与数据恢复

首先，需要已经配置了Standby节点并至少进行过一次数据的全量备份与增量备份。当要执行恢复与扩容时，需要在所需要container中依次执行以下步骤

- 运行tools/download.py full，以从volume backup中获得最新的数据备份压缩包
- 运行tools/download.py increment，以从volume backup中获得用于增量备份的所有binlog文件
- 清空mysql文件夹(rm -rf /var/lib/mysql/\*)，创建触发数据全量恢复的条件
- 重启该container，由于执行了第一步和第三步，因此会触发full_recover.sh脚本。此时会将数据恢复到全量备份的位置，并在mysql_backup文件夹中留下记录全量备份点和binlog文件位置的对应关系的文件，为后面的增量恢复做准备。
- 进入container，手动运行/lain/app/tools/incr_recover.sh脚本。此时会执行增量恢复，并将数据恢复到上一次增量备份成功的位置
- 数据恢复成功后，在Monitor的web控制台中将相应的mysql instance注册为slave，并Active为master的slave。此时slave会继续恢复从上次增量备份成功到现在的数据。当状态为OK时，扩容或数据恢复即完成

### 3.3 密码与secret.conf文件

conf/secret.conf文件中保存了mysql-service的ClientId，Secret，MySQL root账户和repl账户的密码。

**该文件仅需要在首次部署前配置，之后请不要修改，否则会影响集群管理与身份验证功能**

secrent.conf中配置格式为 *key*=*value*，请不要在等号两侧加空格。

目前的配置项有:
- root_passwd: MySQL的root密码
- repl_passwd: MySQL管理集群状态的repl用户密码，请不要修改
- client_id: SSO中注册的mysql-service app id
- secret: SSO注册mysql-service app时的秘密

secret.conf会保存在secret_files中，不会出现在代码库中。
