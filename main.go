package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"os"
)

func main() {
	app := &cli.App{
		Name:    "minx",
		Usage:   "Minio Storage Command Tool",
		Version: "0.1.0",
		Commands: []*cli.Command{
			{
				Name:   "login",
				Usage:  "登录到 Minio 服务器",
				Action: loginAction,
			},
			{
				Name:   "logout",
				Usage:  "退出当前会话",
				Action: logoutAction,
			},
			{
				Name:   "sessions",
				Usage:  "列出所有会话",
				Action: sessionsAction,
			},
			{
				Name:   "switch",
				Usage:  "切换会话",
				Action: switchAction,
			},
			{
				Name:   "info",
				Usage:  "显示当前会话信息",
				Action: infoAction,
			},
			{
				Name:  "ls",
				Usage: "列出目录内容",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "d",
						Usage: "仅显示目录",
					},
					&cli.BoolFlag{
						Name:  "r",
						Usage: "按修改时间倒序排列",
					},
					&cli.BoolFlag{
						Name:  "color",
						Usage: "彩色输出",
					},
					&cli.IntFlag{
						Name:  "c",
						Usage: "显示前 N 个文件或目录",
					},
				},
				Action: lsAction,
			},
			{
				Name:   "cd",
				Usage:  "改变工作目录",
				Action: cdAction,
			},
			{
				Name:   "pwd",
				Usage:  "显示当前工作目录",
				Action: pwdAction,
			},
			{
				Name:   "mkdir",
				Usage:  "创建目录",
				Action: mkdirAction,
			},
			{
				Name:   "tree",
				Usage:  "显示目录结构",
				Action: treeAction,
			},
			{
				Name:  "get",
				Usage: "下载文件或目录",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "w",
						Aliases: []string{"workers"},
						Usage:   "并发下载线程数 (1-10)",
						Value:   5,
					},
					&cli.BoolFlag{
						Name:    "c",
						Aliases: []string{"continue"},
						Usage:   "断点续传",
					},
					&cli.StringFlag{
						Name:  "start",
						Usage: "起始文件名（按字典序）",
					},
					&cli.StringFlag{
						Name:  "end",
						Usage: "结束文件名（按字典序）",
					},
				},
				Action: getAction,
			},
			{
				Name:  "put",
				Usage: "上传文件或目录",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "w",
						Aliases: []string{"workers"},
						Usage:   "并发上传线程数 (1-10)",
						Value:   5,
					},
					&cli.BoolFlag{
						Name:  "all",
						Usage: "包含隐藏文件和目录",
					},
				},
				Action: putAction,
			},
			{
				Name:  "upload",
				Usage: "上传多个文件或目录",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "w",
						Aliases: []string{"workers"},
						Usage:   "并发上传线程数 (1-10)",
						Value:   5,
					},
					&cli.BoolFlag{
						Name:  "all",
						Usage: "包含隐藏文件和目录",
					},
					&cli.StringFlag{
						Name:  "remote",
						Usage: "远程目标路径",
					},
					&cli.StringFlag{
						Name:  "err-log",
						Usage: "错误日志文件",
					},
				},
				Action: uploadAction,
			},
			{
				Name:  "rm",
				Usage: "删除文件或目录",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "d",
						Usage: "仅删除目录",
					},
					&cli.BoolFlag{
						Name:  "a",
						Usage: "删除文件和目录",
					},
					&cli.BoolFlag{
						Name:  "async",
						Usage: "异步删除",
					},
				},
				Action: rmAction,
			},
			{
				Name:  "mv",
				Usage: "移动文件",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "f",
						Usage: "允许覆盖目标文件",
					},
				},
				Action: mvAction,
			},
			{
				Name:  "cp",
				Usage: "复制文件",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "f",
						Usage: "允许覆盖目标文件",
					},
				},
				Action: cpAction,
			},
			{
				Name:  "sync",
				Usage: "同步本地目录到远程",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "w",
						Aliases: []string{"workers"},
						Usage:   "并发线程数",
						Value:   5,
					},
					&cli.BoolFlag{
						Name:  "delete",
						Usage: "删除本地不存在的远程文件",
					},
				},
				Action: syncAction,
			},
			{
				Name:   "auth",
				Usage:  "生成认证字符串",
				Action: authAction,
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "不显示详细信息",
			},
			&cli.StringFlag{
				Name:  "auth",
				Usage: "认证字符串 (endpoint:accessKey:secretKey:bucketName)",
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}
