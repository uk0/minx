

#### quick start


```bash

make

```


```bash


❯ ./minx                                        
NAME:
   minx - Minio Storage Command Tool

USAGE:
   minx [global options] command [command options]

VERSION:
   0.1.0

COMMANDS:
   login     登录到 Minio 服务器
   logout    退出当前会话
   sessions  列出所有会话
   switch    切换会话
   info      显示当前会话信息
   ls        列出目录内容
   cd        改变工作目录
   pwd       显示当前工作目录
   mkdir     创建目录
   tree      显示目录结构
   get       下载文件或目录
   put       上传文件或目录
   upload    上传多个文件或目录
   rm        删除文件或目录
   mv        移动文件
   cp        复制文件
   sync      同步本地目录到远程
   auth      生成认证字符串
   help, h   Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --quiet, -q    不显示详细信息 (default: false)
   --auth value   认证字符串 (endpoint:accessKey:secretKey:bucketName)
   --help, -h     show help
   --version, -v  print the version


```