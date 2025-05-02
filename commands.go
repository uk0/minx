package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"minx/wildcard"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/minio/minio-go/v7"
	"github.com/urfave/cli/v2"
)

// 登录操作
func loginAction(c *cli.Context) error {
	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Endpoint (例如 https(http)://play.min.io): ")
	endpoint, _ := reader.ReadString('\n')
	endpoint = strings.TrimSpace(endpoint)

	// 检查是否包含 http:// 或 https:// 前缀
	secure := true
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
		secure = false
	} else if strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	fmt.Print("Access Key: ")
	accessKey, _ := reader.ReadString('\n')
	accessKey = strings.TrimSpace(accessKey)

	fmt.Print("Secret Key: ")
	secretKey, _ := reader.ReadString('\n')
	secretKey = strings.TrimSpace(secretKey)

	fmt.Print("Bucket 名称: ")
	bucketName, _ := reader.ReadString('\n')
	bucketName = strings.TrimSpace(bucketName)

	// 验证连接
	fmt.Println("验证连接中...")
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure, // 根据用户输入的协议决定是否使用 HTTPS
	})
	if err != nil {
		return fmt.Errorf("创建客户端失败: %w", err)
	}

	exists, err := client.BucketExists(context.Background(), bucketName)
	if err != nil {
		return fmt.Errorf("验证 bucket 失败: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket '%s' 不存在", bucketName)
	}

	// 格式化 endpoint 以供显示和存储
	displayEndpoint := endpoint
	if secure {
		displayEndpoint = "https://" + endpoint
	} else {
		displayEndpoint = "http://" + endpoint
	}

	// 保存会话
	sessionName := fmt.Sprintf("%s/%s", bucketName, displayEndpoint)
	session := Session{
		Endpoint:    displayEndpoint,
		AccessKey:   accessKey,
		SecretKey:   secretKey,
		BucketName:  bucketName,
		CurrentPath: "/",
	}

	if err := manager.AddSession(sessionName, session); err != nil {
		return err
	}

	if err := manager.SwitchSession(sessionName); err != nil {
		return err
	}

	fmt.Printf("登录成功! 当前会话: %s\n", sessionName)
	return nil
}

// 退出登录操作
func logoutAction(c *cli.Context) error {
	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	if manager.CurrentName == "" {
		return fmt.Errorf("没有活动会话")
	}

	currentName := manager.CurrentName
	if err := manager.RemoveSession(currentName); err != nil {
		return err
	}

	fmt.Printf("已退出会话: %s\n", currentName)
	return nil
}

// 列出会话操作
func sessionsAction(c *cli.Context) error {
	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	if len(manager.Sessions) == 0 {
		fmt.Println("没有保存的会话")
		return nil
	}

	fmt.Println("已保存的会话:")
	for name := range manager.Sessions {
		prefix := "  "
		if name == manager.CurrentName {
			prefix = "> "
		}
		fmt.Printf("%s%s\n", prefix, name)
	}

	return nil
}

// 切换会话操作
func switchAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("需要指定会话名称")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	sessionName := c.Args().First()
	if err := manager.SwitchSession(sessionName); err != nil {
		return err
	}

	fmt.Printf("已切换到会话: %s\n", sessionName)
	return nil
}

// 显示当前会话信息
func infoAction(c *cli.Context) error {
	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	fmt.Printf("当前会话信息:\n")
	fmt.Printf("  Endpoint:     %s\n", session.Endpoint)
	fmt.Printf("  Bucket:       %s\n", session.BucketName)
	fmt.Printf("  Access Key:   %s\n", session.AccessKey)
	fmt.Printf("  Current Path: %s\n", session.CurrentPath)

	// 检查桶是否存在
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, session.BucketName)
	if err != nil {
		fmt.Printf("  无法连接到服务器: %v\n", err)
	} else if !exists {
		fmt.Printf("  警告: Bucket '%s' 不存在\n", session.BucketName)
	} else {
		fmt.Printf("  状态:         Bucket 存在且可访问\n")

		// 获取一些基本统计信息
		// 注意：MinIO 不直接提供桶大小，我们可以列出一些对象获取基本统计信息
		objChan := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
			Recursive: true,
			MaxKeys:   1000, // 限制数量以避免性能问题
		})

		var totalSize int64
		var objectCount int
		for obj := range objChan {
			if obj.Err != nil {
				continue
			}
			totalSize += obj.Size
			objectCount++
		}

		if objectCount >= 1000 {
			fmt.Printf("  对象数量:     >1000 个对象\n")
		} else {
			fmt.Printf("  对象数量:     %d 个对象\n", objectCount)
		}
		fmt.Printf("  已用空间:     %s\n", formatSize(totalSize))
	}

	return nil
}

// 列出文件操作
func lsAction(c *cli.Context) error {
	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	var remotePathArg string
	if c.NArg() > 0 {
		remotePathArg = c.Args().First()
	}

	remotePath, err := manager.FormatPath(remotePathArg)
	if err != nil {
		return err
	}

	// 确保路径以 / 结尾
	prefix := strings.TrimPrefix(remotePath, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// 列出对象
	ctx := context.Background()
	objectCh := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	})

	// 收集目录和文件
	var directories []string
	var files []minio.ObjectInfo

	for object := range objectCh {
		if object.Err != nil {
			return fmt.Errorf("列出对象时出错: %w", object.Err)
		}

		name := strings.TrimPrefix(object.Key, prefix)
		if strings.Contains(name, "/") {
			// 这是一个目录
			dirName := strings.Split(name, "/")[0] + "/"
			if !contains(directories, dirName) {
				directories = append(directories, dirName)
			}
		} else if name != "" {
			// 这是一个文件
			files = append(files, object)
		}
	}

	// 按修改时间排序（如果需要）
	if c.Bool("r") {
		sort.Slice(files, func(i, j int) bool {
			return files[i].LastModified.After(files[j].LastModified)
		})
	} else {
		sort.Slice(files, func(i, j int) bool {
			return files[i].LastModified.Before(files[j].LastModified)
		})
	}

	// 限制输出数量（如果需要）
	limit := c.Int("c")
	count := 0

	// 输出目录
	if !c.Bool("d") || len(files) == 0 {
		for _, dir := range directories {
			if limit > 0 && count >= limit {
				break
			}

			if c.Bool("color") {
				color.New(color.FgBlue, color.Bold).Printf("%s\n", dir)
			} else {
				fmt.Printf("%s\n", dir)
			}
			count++
		}
	}

	// 输出文件
	if !c.Bool("d") {
		for _, file := range files {
			if limit > 0 && count >= limit {
				break
			}

			name := strings.TrimPrefix(file.Key, prefix)
			sizeStr := formatSize(file.Size)
			timeStr := file.LastModified.Format("2006-01-02 15:04:05")

			if c.Bool("color") {
				fmt.Printf("%s  %8s  %s\n", timeStr, sizeStr, name)
			} else {
				fmt.Printf("%s  %8s  %s\n", timeStr, sizeStr, name)
			}
			count++
		}
	}

	return nil
}

// 辅助函数：检查字符串是否在切片中
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// 辅助函数：格式化文件大小
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

// 改变目录操作
func cdAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("需要指定目标路径")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	path := c.Args().First()

	var newPath string
	if path == ".." {
		// 返回上一级目录
		if session.CurrentPath == "/" {
			return nil // 已经在根目录
		}
		parts := strings.Split(strings.TrimSuffix(session.CurrentPath, "/"), "/")
		if len(parts) <= 1 {
			newPath = "/"
		} else {
			newPath = "/" + strings.Join(parts[:len(parts)-1], "/")
		}
	} else {
		// 格式化路径
		formattedPath, err := manager.FormatPath(path)
		if err != nil {
			return err
		}
		newPath = formattedPath
	}

	// 验证目录是否存在 (在 Minio 中通过检查是否有以该前缀开头的对象)
	if newPath != "/" {
		client, err := manager.GetClient()
		if err != nil {
			return err
		}

		// 确保路径以 / 结尾
		prefix := strings.TrimPrefix(newPath, "/")
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}

		// 列出对象
		ctx := context.Background()
		objectCh := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: false,
			MaxKeys:   1,
		})

		exists := false
		for range objectCh {
			exists = true
			break
		}

		if !exists {
			return fmt.Errorf("目录 '%s' 不存在", newPath)
		}
	}

	// 更新当前路径
	if err := manager.UpdateCurrentPath(newPath); err != nil {
		return err
	}

	fmt.Printf("当前目录: %s\n", newPath)
	return nil
}

// 显示当前路径操作
func pwdAction(c *cli.Context) error {
	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	fmt.Println(session.CurrentPath)
	return nil
}

// 创建目录操作
func mkdirAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("需要指定目录名称")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	dirName := c.Args().First()
	remotePath, err := manager.FormatPath(dirName)
	if err != nil {
		return err
	}

	// 确保路径以 / 结尾
	objectName := strings.TrimPrefix(remotePath, "/")
	if objectName != "" && !strings.HasSuffix(objectName, "/") {
		objectName += "/"
	}

	// 在 Minio 中，目录是一个空对象，名称以 / 结尾
	ctx := context.Background()
	_, err = client.PutObject(ctx, session.BucketName, objectName, strings.NewReader(""), 0, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	fmt.Printf("目录 '%s' 创建成功\n", remotePath)
	return nil
}

// 显示目录结构操作
func treeAction(c *cli.Context) error {
	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	var remotePathArg string
	if c.NArg() > 0 {
		remotePathArg = c.Args().First()
	}

	remotePath, err := manager.FormatPath(remotePathArg)
	if err != nil {
		return err
	}

	// 确保路径以 / 结尾
	prefix := strings.TrimPrefix(remotePath, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// 递归列出所有对象
	ctx := context.Background()
	objectCh := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	// 构建目录树
	tree := make(map[string][]string)
	var allPaths []string

	for object := range objectCh {
		if object.Err != nil {
			return fmt.Errorf("列出对象时出错: %w", object.Err)
		}

		path := strings.TrimPrefix(object.Key, prefix)
		if path == "" {
			continue
		}

		allPaths = append(allPaths, path)

		// 构建目录结构
		parts := strings.Split(path, "/")
		for i := 0; i < len(parts); i++ {
			if i == len(parts)-1 && !strings.HasSuffix(path, "/") {
				// 这是文件
				parent := strings.Join(parts[:i], "/")
				if parent == "" {
					parent = "/"
				}
				tree[parent] = append(tree[parent], parts[i])
			} else {
				// 这是目录
				dirPath := strings.Join(parts[:i+1], "/")
				parentPath := "/"
				if i > 0 {
					parentPath = strings.Join(parts[:i], "/")
				}

				// 确保目录存在于树中
				if _, exists := tree[dirPath]; !exists {
					tree[dirPath] = []string{}
				}

				// 将目录添加到父目录中
				dirName := parts[i] + "/"
				if !containsStr(tree[parentPath], dirName) {
					tree[parentPath] = append(tree[parentPath], dirName)
				}
			}
		}
	}

	// 打印目录树
	fmt.Printf("%s\n", remotePath)
	printTree(tree, "/", 0)

	return nil
}

// 辅助函数：递归打印目录树
func printTree(tree map[string][]string, currentPath string, depth int) {
	items := tree[currentPath]
	sort.Strings(items)

	for i, item := range items {
		isLast := i == len(items)-1
		prefix := ""
		for j := 0; j < depth; j++ {
			prefix += "│   "
		}

		if isLast {
			fmt.Printf("%s└── %s\n", prefix, item)
		} else {
			fmt.Printf("%s├── %s\n", prefix, item)
		}

		if strings.HasSuffix(item, "/") {
			// 这是目录，递归打印
			nextPath := currentPath
			if nextPath != "/" {
				nextPath += "/"
			}
			nextPath += strings.TrimSuffix(item, "/")
			printTree(tree, nextPath, depth+1)
		}
	}
}

// 辅助函数：检查字符串是否在切片中
func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// 下载文件或目录操作
func getAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("需要指定远程文件或目录")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	remotePath := c.Args().First()
	formattedPath, err := manager.FormatPath(remotePath)
	if err != nil {
		return err
	}

	objectName := strings.TrimPrefix(formattedPath, "/")

	// 确定本地保存路径
	var localPath string
	if c.NArg() > 1 {
		localPath = c.Args().Get(1)
	} else {
		// 使用远程文件名作为本地文件名
		parts := strings.Split(objectName, "/")
		localPath = parts[len(parts)-1]
		if localPath == "" {
			localPath = filepath.Base(objectName)
		}
	}

	// 判断是文件还是目录
	ctx := context.Background()
	isDir := strings.HasSuffix(objectName, "/")
	if !isDir {
		// 检查是否存在该文件
		objInfo, err := client.StatObject(ctx, session.BucketName, objectName, minio.StatObjectOptions{})
		if err != nil {
			// 检查是否是目录
			objects := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
				Prefix:    objectName + "/",
				Recursive: false,
				MaxKeys:   1,
			})

			hasObjects := false
			for range objects {
				hasObjects = true
				break
			}

			if hasObjects {
				isDir = true
				objectName += "/"
			} else {
				return fmt.Errorf("文件或目录 '%s' 不存在", formattedPath)
			}
		} else {
			// 是文件，准备下载
			fmt.Printf("下载文件: %s (%s)\n", formattedPath, formatSize(objInfo.Size))

			// 创建目录
			localDir := filepath.Dir(localPath)
			if localDir != "." {
				if err := os.MkdirAll(localDir, 0755); err != nil {
					return fmt.Errorf("创建本地目录失败: %w", err)
				}
			}

			// 执行下载
			if c.Bool("c") && fileExists(localPath) {
				// 断点续传
				fileInfo, err := os.Stat(localPath)
				if err != nil {
					return fmt.Errorf("获取本地文件信息失败: %w", err)
				}

				if fileInfo.Size() >= objInfo.Size {
					fmt.Printf("文件已完成下载: %s\n", localPath)
					return nil
				}

				fmt.Printf("继续下载文件: %s (从 %s/%s)\n",
					localPath, formatSize(fileInfo.Size()), formatSize(objInfo.Size))

				opts := minio.GetObjectOptions{}
				opts.SetRange(fileInfo.Size(), objInfo.Size-1)

				// 打开本地文件进行追加
				file, err := os.OpenFile(localPath, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return fmt.Errorf("打开本地文件失败: %w", err)
				}
				defer file.Close()

				// 下载剩余部分
				obj, err := client.GetObject(ctx, session.BucketName, objectName, opts)
				if err != nil {
					return fmt.Errorf("获取对象失败: %w", err)
				}
				defer obj.Close()

				// 创建进度条
				progress := &ProgressBar{
					Total:     objInfo.Size,
					Current:   fileInfo.Size(),
					Width:     50,
					FileName:  localPath,
					StartTime: time.Now(),
				}

				// 创建缓冲读取器
				reader := NewProgressReader(obj, progress)

				// 复制数据到文件
				written, err := io.Copy(file, reader)
				if err != nil {
					return fmt.Errorf("下载文件失败: %w", err)
				}

				fmt.Printf("\n已下载 %s 字节到 %s\n", formatSize(fileInfo.Size()+written), localPath)

			} else {
				// 常规下载
				file, err := os.Create(localPath)
				if err != nil {
					return fmt.Errorf("创建本地文件失败: %w", err)
				}
				defer file.Close()

				// 创建进度条
				progress := &ProgressBar{
					Total:     objInfo.Size,
					Width:     50,
					FileName:  localPath,
					StartTime: time.Now(),
				}

				// 获取对象
				obj, err := client.GetObject(ctx, session.BucketName, objectName, minio.GetObjectOptions{})
				if err != nil {
					return fmt.Errorf("获取对象失败: %w", err)
				}
				defer obj.Close()

				// 创建缓冲读取器
				reader := NewProgressReader(obj, progress)

				// 复制数据到文件
				written, err := io.Copy(file, reader)
				if err != nil {
					return fmt.Errorf("下载文件失败: %w", err)
				}

				fmt.Printf("\n已下载 %s 字节到 %s\n", formatSize(written), localPath)
			}

			return nil
		}
	}

	if isDir {
		// 下载目录
		fmt.Printf("下载目录: %s 到 %s\n", formattedPath, localPath)

		// 创建本地目录
		if err := os.MkdirAll(localPath, 0755); err != nil {
			return fmt.Errorf("创建本地目录失败: %w", err)
		}

		// 列出目录内所有对象
		prefix := objectName
		objects := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		})

		// 并发下载限制
		workers := c.Int("w")
		if workers < 1 {
			workers = 1
		} else if workers > 10 {
			workers = 10
		}

		// 创建工作池
		var wg sync.WaitGroup
		jobCh := make(chan minio.ObjectInfo)

		// 启动工作线程
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for obj := range jobCh {
					// 跳过目录对象
					if strings.HasSuffix(obj.Key, "/") {
						continue
					}

					// 从对象路径中提取相对路径
					relPath := strings.TrimPrefix(obj.Key, prefix)

					// 确定本地文件路径
					filePath := filepath.Join(localPath, relPath)

					// 创建目录结构
					if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
						fmt.Fprintf(os.Stderr, "无法创建目录 '%s': %v\n", filepath.Dir(filePath), err)
						continue
					}

					// 检查是否需要跳过
					if c.String("start") != "" && relPath < c.String("start") {
						continue
					}
					if c.String("end") != "" && relPath >= c.String("end") {
						continue
					}

					// 下载文件
					if c.Bool("c") && fileExists(filePath) {
						// 断点续传
						fileInfo, err := os.Stat(filePath)
						if err != nil {
							fmt.Fprintf(os.Stderr, "获取文件信息失败 '%s': %v\n", filePath, err)
							continue
						}

						if fileInfo.Size() >= obj.Size {
							fmt.Printf("文件已完成下载: %s\n", filePath)
							continue
						}

						fmt.Printf("继续下载: %s (%s/%s)\n",
							filePath, formatSize(fileInfo.Size()), formatSize(obj.Size))

						opts := minio.GetObjectOptions{}
						opts.SetRange(fileInfo.Size(), obj.Size-1)

						// 打开本地文件进行追加
						file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
						if err != nil {
							fmt.Fprintf(os.Stderr, "打开文件失败 '%s': %v\n", filePath, err)
							continue
						}

						// 下载剩余部分
						objReader, err := client.GetObject(ctx, session.BucketName, obj.Key, opts)
						if err != nil {
							fmt.Fprintf(os.Stderr, "获取对象失败 '%s': %v\n", obj.Key, err)
							file.Close()
							continue
						}

						// 复制数据到文件
						written, err := io.Copy(file, objReader)
						objReader.Close()
						file.Close()

						if err != nil {
							fmt.Fprintf(os.Stderr, "下载文件失败 '%s': %v\n", filePath, err)
							continue
						}

						fmt.Printf("已下载: %s (%s 字节)\n", filePath, formatSize(fileInfo.Size()+written))

					} else {
						// 常规下载
						fmt.Printf("下载: %s (%s)\n", filePath, formatSize(obj.Size))

						file, err := os.Create(filePath)
						if err != nil {
							fmt.Fprintf(os.Stderr, "创建文件失败 '%s': %v\n", filePath, err)
							continue
						}

						// 获取对象
						objReader, err := client.GetObject(ctx, session.BucketName, obj.Key, minio.GetObjectOptions{})
						if err != nil {
							fmt.Fprintf(os.Stderr, "获取对象失败 '%s': %v\n", obj.Key, err)
							file.Close()
							continue
						}

						// 复制数据到文件
						written, err := io.Copy(file, objReader)
						objReader.Close()
						file.Close()

						if err != nil {
							fmt.Fprintf(os.Stderr, "下载文件失败 '%s': %v\n", filePath, err)
							continue
						}

						fmt.Printf("已下载: %s (%s 字节)\n", filePath, formatSize(written))
					}
				}
			}()
		}

		// 发送下载任务
		for obj := range objects {
			if obj.Err != nil {
				fmt.Fprintf(os.Stderr, "列出对象时出错: %v\n", obj.Err)
				continue
			}
			jobCh <- obj
		}

		// 关闭任务通道并等待所有工作线程完成
		close(jobCh)
		wg.Wait()

		fmt.Printf("目录下载完成: %s\n", localPath)
	}

	return nil
}

// 辅助函数：检查文件是否存在
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// 上传文件操作
func putAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("需要指定本地文件或目录")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	localPath := c.Args().First()

	// 检查本地路径是否是 URL
	isURL := strings.HasPrefix(localPath, "http://") || strings.HasPrefix(localPath, "https://")

	// 确定远程保存路径
	var remotePath string
	if c.NArg() > 1 {
		remotePath = c.Args().Get(1)
	} else if isURL {
		// 从 URL 提取文件名
		parts := strings.Split(localPath, "/")
		remotePath = parts[len(parts)-1]
	} else {
		// 使用本地文件名作为远程文件名
		remotePath = filepath.Base(localPath)
	}

	formattedPath, err := manager.FormatPath(remotePath)
	if err != nil {
		return err
	}

	objectName := strings.TrimPrefix(formattedPath, "/")

	// 上传文件或目录
	ctx := context.Background()

	if isURL {
		// 从 URL 上传
		fmt.Printf("从 URL 上传: %s 到 %s\n", localPath, formattedPath)

		// 获取 URL 内容
		resp, err := http.Get(localPath)
		if err != nil {
			return fmt.Errorf("获取 URL 内容失败: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP 请求失败: %s", resp.Status)
		}

		// 计算内容大小
		contentLength := resp.ContentLength

		// 创建进度条
		progress := &ProgressBar{
			Total:     contentLength,
			Width:     50,
			FileName:  objectName,
			StartTime: time.Now(),
		}

		// 创建带进度的读取器
		reader := NewProgressReader(resp.Body, progress)

		// 执行上传
		contentType := resp.Header.Get("Content-Type")
		_, err = client.PutObject(ctx, session.BucketName, objectName, reader, contentLength, minio.PutObjectOptions{
			ContentType: contentType,
		})
		if err != nil {
			return fmt.Errorf("上传文件失败: %w", err)
		}

		fmt.Printf("\n已上传 %s 字节到 %s\n", formatSize(contentLength), formattedPath)

	} else {
		// 从本地文件上传
		fileInfo, err := os.Stat(localPath)
		if err != nil {
			return fmt.Errorf("获取文件信息失败: %w", err)
		}

		if fileInfo.IsDir() {
			// 目录上传
			fmt.Printf("上传目录: %s 到 %s\n", localPath, formattedPath)

			// 确保远程路径是目录
			if !strings.HasSuffix(objectName, "/") {
				objectName += "/"
			}

			// 创建远程目录
			_, err = client.PutObject(ctx, session.BucketName, objectName, strings.NewReader(""), 0, minio.PutObjectOptions{})
			if err != nil {
				return fmt.Errorf("创建远程目录失败: %w", err)
			}

			// 递归上传目录内容
			err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// 跳过隐藏文件和目录 (除非指定了 --all 选项)
				if !c.Bool("all") && strings.HasPrefix(filepath.Base(path), ".") {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				// 计算相对路径
				relPath, err := filepath.Rel(localPath, path)
				if err != nil {
					return err
				}

				// 转换 Windows 路径分隔符
				relPath = strings.ReplaceAll(relPath, "\\", "/")

				if info.IsDir() {
					// 创建目录
					dirObjectName := objectName
					if relPath != "." {
						dirObjectName += relPath + "/"
					}

					_, err = client.PutObject(ctx, session.BucketName, dirObjectName, strings.NewReader(""), 0, minio.PutObjectOptions{})
					if err != nil {
						fmt.Fprintf(os.Stderr, "创建目录失败 '%s': %v\n", dirObjectName, err)
					}
				} else {
					// 上传文件
					fileObjectName := objectName + relPath

					fmt.Printf("上传: %s (%s)\n", path, formatSize(info.Size()))

					file, err := os.Open(path)
					if err != nil {
						fmt.Fprintf(os.Stderr, "无法打开文件 '%s': %v\n", path, err)
						return nil
					}
					defer file.Close()

					// 获取文件 MIME 类型
					contentType := getMimeType(path)

					// 执行上传
					_, err = client.PutObject(ctx, session.BucketName, fileObjectName, file, info.Size(), minio.PutObjectOptions{
						ContentType: contentType,
					})
					if err != nil {
						fmt.Fprintf(os.Stderr, "上传文件失败 '%s': %v\n", path, err)
					}
				}

				return nil
			})

			if err != nil {
				return fmt.Errorf("遍历目录失败: %w", err)
			}

			fmt.Printf("目录上传完成: %s\n", formattedPath)

		} else {
			// 文件上传
			fmt.Printf("上传文件: %s (%s) 到 %s\n", localPath, formatSize(fileInfo.Size()), formattedPath)

			file, err := os.Open(localPath)
			if err != nil {
				return fmt.Errorf("无法打开文件: %w", err)
			}
			defer file.Close()

			// 创建进度条
			progress := &ProgressBar{
				Total:     fileInfo.Size(),
				Width:     50,
				FileName:  localPath,
				StartTime: time.Now(),
			}

			// 创建带进度的读取器
			reader := NewProgressReader(file, progress)

			// 获取文件 MIME 类型
			contentType := getMimeType(localPath)

			// 执行上传
			_, err = client.PutObject(ctx, session.BucketName, objectName, reader, fileInfo.Size(), minio.PutObjectOptions{
				ContentType: contentType,
			})
			if err != nil {
				return fmt.Errorf("上传文件失败: %w", err)
			}

			fmt.Printf("\n已上传 %s 字节到 %s\n", formatSize(fileInfo.Size()), formattedPath)
		}
	}

	return nil
}

// 辅助函数：获取文件 MIME 类型
func getMimeType(path string) string {
	ext := filepath.Ext(path)
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

// 上传多个文件操作
func uploadAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("需要指定本地文件或目录或匹配模式")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	// 获取远程目标路径
	remotePath := c.String("remote")
	if remotePath == "" {
		remotePath = session.CurrentPath
	}

	// 处理远程路径（可以是相对或绝对路径）
	formattedPath, err := manager.FormatPath(remotePath)
	if err != nil {
		return err
	}

	// 确保路径以 / 结尾
	objectPrefix := strings.TrimPrefix(formattedPath, "/")
	if objectPrefix != "" && !strings.HasSuffix(objectPrefix, "/") {
		objectPrefix += "/"
	}

	// 收集要上传的文件
	var filesToUpload []string
	for i := 0; i < c.NArg(); i++ {
		pattern := c.Args().Get(i)

		// 处理本地相对路径模式
		if !filepath.IsAbs(pattern) && !strings.Contains(pattern, "*") {
			currentDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("获取当前工作目录失败: %w", err)
			}
			pattern = filepath.Join(currentDir, pattern)
		}

		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "无效的匹配模式 '%s': %v\n", pattern, err)
			continue
		}

		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "没有匹配文件: %s\n", pattern)
			continue
		}

		for _, match := range matches {
			filesToUpload = append(filesToUpload, match)
		}
	}

	if len(filesToUpload) == 0 {
		return fmt.Errorf("没有找到要上传的文件")
	}

	// 设置错误日志
	var errLog *os.File
	if c.String("err-log") != "" {
		errLog, err = os.Create(c.String("err-log"))
		if err != nil {
			return fmt.Errorf("创建错误日志文件失败: %w", err)
		}
		defer errLog.Close()
	}

	// 并发上传限制
	workers := c.Int("w")
	if workers < 1 {
		workers = 1
	} else if workers > 10 {
		workers = 10
	}

	// 创建工作池
	var wg sync.WaitGroup
	jobCh := make(chan string)

	// 启动工作线程
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()

			for localPath := range jobCh {
				// 确保使用绝对路径
				if !filepath.IsAbs(localPath) {
					currentDir, err := os.Getwd()
					if err != nil {
						logError(errLog, "获取当前工作目录失败: %v", err)
						continue
					}
					localPath = filepath.Join(currentDir, localPath)
				}

				fileInfo, err := os.Stat(localPath)
				if err != nil {
					logError(errLog, "获取文件信息失败 '%s': %v", localPath, err)
					continue
				}

				if fileInfo.IsDir() {
					// 目录上传
					fmt.Printf("上传目录: %s -> %s\n", localPath, formattedPath+filepath.Base(localPath)+"/")

					// 计算目录名
					dirName := filepath.Base(localPath)
					dirObjectPrefix := objectPrefix + dirName + "/"

					// 创建远程目录
					_, err = client.PutObject(ctx, session.BucketName, dirObjectPrefix, strings.NewReader(""), 0, minio.PutObjectOptions{})
					if err != nil {
						logError(errLog, "创建远程目录失败 '%s': %v", dirObjectPrefix, err)
						continue
					}

					// 递归上传目录内容
					err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
						if err != nil {
							return err
						}

						// 跳过隐藏文件和目录 (除非指定了 --all 选项)
						if !c.Bool("all") && strings.HasPrefix(filepath.Base(path), ".") {
							if info.IsDir() {
								return filepath.SkipDir
							}
							return nil
						}

						// 计算相对路径
						relPath, err := filepath.Rel(localPath, path)
						if err != nil {
							return err
						}

						// 转换 Windows 路径分隔符
						relPath = strings.ReplaceAll(relPath, "\\", "/")

						if info.IsDir() {
							// 创建目录
							if relPath != "." {
								dirObjName := dirObjectPrefix + relPath + "/"
								_, err = client.PutObject(ctx, session.BucketName, dirObjName, strings.NewReader(""), 0, minio.PutObjectOptions{})
								if err != nil {
									logError(errLog, "创建目录失败 '%s': %v", dirObjName, err)
								}
							}
						} else {
							// 上传文件
							fileObjectName := dirObjectPrefix + relPath

							fmt.Printf("上传: %s (%s)\n", path, formatSize(info.Size()))

							file, err := os.Open(path)
							if err != nil {
								logError(errLog, "无法打开文件 '%s': %v", path, err)
								return nil
							}

							// 创建进度条
							progress := &ProgressBar{
								Total:     info.Size(),
								Width:     50,
								FileName:  filepath.Base(path),
								StartTime: time.Now(),
							}

							// 创建带进度的读取器
							reader := NewProgressReader(file, progress)

							// 获取文件 MIME 类型
							contentType := getMimeType(path)

							// 执行上传
							_, err = client.PutObject(ctx, session.BucketName, fileObjectName, reader, info.Size(), minio.PutObjectOptions{
								ContentType: contentType,
							})

							file.Close()

							fmt.Println() // 进度条完成后换行

							if err != nil {
								logError(errLog, "上传文件失败 '%s': %v", path, err)
							}
						}

						return nil
					})

					if err != nil {
						logError(errLog, "遍历目录失败 '%s': %v", localPath, err)
					}

				} else {
					// 文件上传
					fmt.Printf("上传文件: %s (%s) -> %s\n", localPath, formatSize(fileInfo.Size()), formattedPath)

					file, err := os.Open(localPath)
					if err != nil {
						logError(errLog, "无法打开文件 '%s': %v", localPath, err)
						continue
					}

					// 计算文件名
					fileName := filepath.Base(localPath)
					fileObjectName := objectPrefix + fileName

					// 创建进度条
					progress := &ProgressBar{
						Total:     fileInfo.Size(),
						Width:     50,
						FileName:  fileName,
						StartTime: time.Now(),
					}

					// 创建带进度的读取器
					reader := NewProgressReader(file, progress)

					// 获取文件 MIME 类型
					contentType := getMimeType(localPath)

					// 执行上传
					_, err = client.PutObject(ctx, session.BucketName, fileObjectName, reader, fileInfo.Size(), minio.PutObjectOptions{
						ContentType: contentType,
					})
					file.Close()

					fmt.Println() // 进度条完成后换行

					if err != nil {
						logError(errLog, "上传文件失败 '%s': %v", localPath, err)
					}
				}
			}
		}()
	}

	// 发送上传任务
	for _, file := range filesToUpload {
		jobCh <- file
	}

	// 关闭任务通道并等待所有工作线程完成
	close(jobCh)
	wg.Wait()

	fmt.Printf("上传完成，共 %d 个文件或目录\n", len(filesToUpload))
	return nil
}

// 辅助函数：记录错误
func logError(file *os.File, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "错误: %s\n", message)

	if file != nil {
		fmt.Fprintf(file, "%s: %s\n", time.Now().Format(time.RFC3339), message)
	}
}

// 删除文件或目录操作
func rmAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("需要指定远程文件或目录")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	remotePath := c.Args().First()
	formattedPath, err := manager.FormatPath(remotePath)
	if err != nil {
		return err
	}

	objectName := strings.TrimPrefix(formattedPath, "/")

	// 检查通配符
	if strings.Contains(objectName, "*") {
		// 有通配符，需要进行匹配
		prefix := objectName[:strings.Index(objectName, "*")]
		pattern := objectName

		// 列出匹配的对象
		ctx := context.Background()
		objectCh := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		})

		var objectsToDelete []string
		for object := range objectCh {
			if object.Err != nil {
				return fmt.Errorf("列出对象时出错: %w", object.Err)
			}

			if wildcard.Match(pattern, object.Key) {
				if c.Bool("d") && !strings.HasSuffix(object.Key, "/") {
					// 跳过不是目录的对象
					continue
				}

				if !c.Bool("a") && !c.Bool("d") && strings.HasSuffix(object.Key, "/") {
					// 跳过目录对象
					continue
				}

				objectsToDelete = append(objectsToDelete, object.Key)
			}
		}

		if len(objectsToDelete) == 0 {
			return fmt.Errorf("没有匹配的文件或目录: %s", formattedPath)
		}

		// 执行删除
		if c.Bool("async") {
			// 异步删除
			go func() {
				for _, obj := range objectsToDelete {
					fmt.Printf("异步删除: %s\n", obj)
					err := client.RemoveObject(context.Background(), session.BucketName, obj, minio.RemoveObjectOptions{})
					if err != nil {
						fmt.Fprintf(os.Stderr, "删除失败 '%s': %v\n", obj, err)
					}
				}
			}()
			fmt.Printf("已启动异步删除 %d 个对象\n", len(objectsToDelete))

		} else {
			// 同步删除
			for _, obj := range objectsToDelete {
				fmt.Printf("删除: %s\n", obj)
				err := client.RemoveObject(ctx, session.BucketName, obj, minio.RemoveObjectOptions{})
				if err != nil {
					fmt.Fprintf(os.Stderr, "删除失败 '%s': %v\n", obj, err)
				}
			}
			fmt.Printf("已删除 %d 个对象\n", len(objectsToDelete))
		}

	} else {
		// 没有通配符，直接删除
		ctx := context.Background()

		// 判断是文件还是目录
		isDir := strings.HasSuffix(objectName, "/")
		if !isDir {
			// 检查是否存在
			_, err := client.StatObject(ctx, session.BucketName, objectName, minio.StatObjectOptions{})
			if err != nil {
				// 检查是否是目录
				objectCh := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
					Prefix:    objectName + "/",
					Recursive: false,
					MaxKeys:   1,
				})

				hasObjects := false
				for range objectCh {
					hasObjects = true
					break
				}

				if hasObjects {
					isDir = true
					objectName += "/"
				} else {
					return fmt.Errorf("文件或目录 '%s' 不存在", formattedPath)
				}
			} else if c.Bool("d") {
				return fmt.Errorf("'%s' 不是目录", formattedPath)
			}
		} else if !c.Bool("a") && !c.Bool("d") {
			return fmt.Errorf("无法删除目录 '%s'，请使用 -d 或 -a 选项", formattedPath)
		}

		if isDir {
			// 删除目录
			if c.Bool("d") || c.Bool("a") {
				// 列出目录下所有对象
				objectCh := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
					Prefix:    objectName,
					Recursive: true,
				})

				var objectsToDelete []string
				for object := range objectCh {
					if object.Err != nil {
						return fmt.Errorf("列出对象时出错: %w", object.Err)
					}
					objectsToDelete = append(objectsToDelete, object.Key)
				}

				if len(objectsToDelete) == 0 {
					fmt.Printf("目录 '%s' 为空\n", formattedPath)
					return nil
				}

				// 执行删除
				if c.Bool("async") {
					// 异步删除
					go func() {
						for _, obj := range objectsToDelete {
							fmt.Printf("异步删除: %s\n", obj)
							err := client.RemoveObject(context.Background(), session.BucketName, obj, minio.RemoveObjectOptions{})
							if err != nil {
								fmt.Fprintf(os.Stderr, "删除失败 '%s': %v\n", obj, err)
							}
						}
					}()
					fmt.Printf("已启动异步删除目录 '%s'，共 %d 个对象\n", formattedPath, len(objectsToDelete))

				} else {
					// 同步删除
					for _, obj := range objectsToDelete {
						fmt.Printf("删除: %s\n", obj)
						err := client.RemoveObject(ctx, session.BucketName, obj, minio.RemoveObjectOptions{})
						if err != nil {
							fmt.Fprintf(os.Stderr, "删除失败 '%s': %v\n", obj, err)
						}
					}
					fmt.Printf("已删除目录 '%s'，共 %d 个对象\n", formattedPath, len(objectsToDelete))
				}
			} else {
				return fmt.Errorf("无法删除目录 '%s'，请使用 -d 或 -a 选项", formattedPath)
			}
		} else {
			// 删除文件
			fmt.Printf("删除文件: %s\n", formattedPath)
			err := client.RemoveObject(ctx, session.BucketName, objectName, minio.RemoveObjectOptions{})
			if err != nil {
				return fmt.Errorf("删除文件失败: %w", err)
			}
			fmt.Printf("已删除文件 '%s'\n", formattedPath)
		}
	}

	return nil
}

// 移动文件操作
func mvAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("需要指定源文件和目标文件")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	sourcePath := c.Args().Get(0)
	destPath := c.Args().Get(1)

	sourceFormatted, err := manager.FormatPath(sourcePath)
	if err != nil {
		return err
	}

	destFormatted, err := manager.FormatPath(destPath)
	if err != nil {
		return err
	}

	sourceObject := strings.TrimPrefix(sourceFormatted, "/")
	destObject := strings.TrimPrefix(destFormatted, "/")

	ctx := context.Background()

	// 检查源对象是否存在
	_, err = client.StatObject(ctx, session.BucketName, sourceObject, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("源文件 '%s' 不存在或无法访问", sourceFormatted)
	}

	// 检查目标对象是否存在
	destExists := false
	_, err = client.StatObject(ctx, session.BucketName, destObject, minio.StatObjectOptions{})
	if err == nil {
		destExists = true
		if !c.Bool("f") {
			return fmt.Errorf("目标文件 '%s' 已存在，使用 -f 选项强制覆盖", destFormatted)
		}
	}

	// 执行复制操作
	fmt.Printf("移动: %s -> %s\n", sourceFormatted, destFormatted)
	_, err = client.CopyObject(ctx, minio.CopyDestOptions{
		Bucket: session.BucketName,
		Object: destObject,
	}, minio.CopySrcOptions{
		Bucket: session.BucketName,
		Object: sourceObject,
	})
	if err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}

	// 删除源对象
	err = client.RemoveObject(ctx, session.BucketName, sourceObject, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("删除源文件失败: %w", err)
	}

	if destExists {
		fmt.Printf("已覆盖移动文件 '%s' 到 '%s'\n", sourceFormatted, destFormatted)
	} else {
		fmt.Printf("已移动文件 '%s' 到 '%s'\n", sourceFormatted, destFormatted)
	}

	return nil
}

// 复制文件操作
func cpAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("需要指定源文件和目标文件")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	sourcePath := c.Args().Get(0)
	destPath := c.Args().Get(1)

	sourceFormatted, err := manager.FormatPath(sourcePath)
	if err != nil {
		return err
	}

	destFormatted, err := manager.FormatPath(destPath)
	if err != nil {
		return err
	}

	sourceObject := strings.TrimPrefix(sourceFormatted, "/")
	destObject := strings.TrimPrefix(destFormatted, "/")

	ctx := context.Background()

	// 检查源对象是否存在
	_, err = client.StatObject(ctx, session.BucketName, sourceObject, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("源文件 '%s' 不存在或无法访问", sourceFormatted)
	}

	// 检查目标对象是否存在
	destExists := false
	_, err = client.StatObject(ctx, session.BucketName, destObject, minio.StatObjectOptions{})
	if err == nil {
		destExists = true
		if !c.Bool("f") {
			return fmt.Errorf("目标文件 '%s' 已存在，使用 -f 选项强制覆盖", destFormatted)
		}
	}

	// 执行复制操作
	fmt.Printf("复制: %s -> %s\n", sourceFormatted, destFormatted)
	_, err = client.CopyObject(ctx, minio.CopyDestOptions{
		Bucket: session.BucketName,
		Object: destObject,
	}, minio.CopySrcOptions{
		Bucket: session.BucketName,
		Object: sourceObject,
	})
	if err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}

	if destExists {
		fmt.Printf("已覆盖复制文件 '%s' 到 '%s'\n", sourceFormatted, destFormatted)
	} else {
		fmt.Printf("已复制文件 '%s' 到 '%s'\n", sourceFormatted, destFormatted)
	}

	return nil
}

// 同步目录操作
func syncAction(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("需要指定本地路径和远程路径")
	}

	manager, err := initSessionManager()
	if err != nil {
		return err
	}

	client, err := manager.GetClient()
	if err != nil {
		return err
	}

	session, err := manager.CurrentSession()
	if err != nil {
		return err
	}

	localPath := c.Args().Get(0)
	remotePath := c.Args().Get(1)

	// 处理本地相对路径
	if !filepath.IsAbs(localPath) {
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前工作目录失败: %w", err)
		}
		localPath = filepath.Join(currentDir, localPath)
	}

	// 处理远程相对路径
	formattedPath, err := manager.FormatPath(remotePath)
	if err != nil {
		return err
	}

	// 检查本地路径
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("无法访问本地路径: %w", err)
	}

	if !fileInfo.IsDir() {
		return fmt.Errorf("本地路径必须是目录")
	}

	// 远程路径处理
	objectPrefix := strings.TrimPrefix(formattedPath, "/")
	if objectPrefix != "" && !strings.HasSuffix(objectPrefix, "/") {
		objectPrefix += "/"
	}

	fmt.Printf("同步目录: %s -> %s\n", localPath, formattedPath)

	ctx := context.Background()

	// 列出远程文件
	remoteFiles := make(map[string]minio.ObjectInfo)

	objectCh := client.ListObjects(ctx, session.BucketName, minio.ListObjectsOptions{
		Prefix:    objectPrefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return fmt.Errorf("列出远程对象时出错: %w", object.Err)
		}

		// 忽略目录对象
		if strings.HasSuffix(object.Key, "/") {
			continue
		}

		relPath := strings.TrimPrefix(object.Key, objectPrefix)
		remoteFiles[relPath] = object
	}

	// 并发上传限制
	workers := c.Int("w")
	if workers < 1 {
		workers = 1
	} else if workers > 10 {
		workers = 10
	}

	// 创建工作池
	var wg sync.WaitGroup
	jobCh := make(chan string)

	// 记录已处理的本地文件，使用互斥锁保护
	processedFiles := make(map[string]bool)
	var processedMutex sync.Mutex

	// 启动工作线程
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for relPath := range jobCh {
				fullLocalPath := filepath.Join(localPath, relPath)
				localFileInfo, err := os.Stat(fullLocalPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "获取本地文件信息失败 '%s': %v\n", fullLocalPath, err)
					continue
				}

				// 跳过目录
				if localFileInfo.IsDir() {
					continue
				}

				// 使用互斥锁保护 map 写入
				processedMutex.Lock()
				processedFiles[relPath] = true
				processedMutex.Unlock()

				remoteObj, exists := remoteFiles[relPath]

				// 上传条件:
				// 1. 远程文件不存在
				// 2. 文件大小不一致
				// 3. 本地文件的修改时间晚于远程文件
				needUpload := false
				if !exists {
					needUpload = true
				} else if localFileInfo.Size() != remoteObj.Size {
					needUpload = true
				} else if localFileInfo.ModTime().After(remoteObj.LastModified) {
					needUpload = true
				}

				if needUpload {
					fmt.Printf("同步: %s\n", relPath)

					file, err := os.Open(fullLocalPath)
					if err != nil {
						fmt.Fprintf(os.Stderr, "无法打开文件 '%s': %v\n", fullLocalPath, err)
						continue
					}

					// 计算对象名
					objectName := objectPrefix + relPath

					// 获取文件 MIME 类型
					contentType := getMimeType(fullLocalPath)

					// 执行上传
					_, err = client.PutObject(ctx, session.BucketName, objectName, file, localFileInfo.Size(), minio.PutObjectOptions{
						ContentType: contentType,
					})
					file.Close()

					if err != nil {
						fmt.Fprintf(os.Stderr, "上传文件失败 '%s': %v\n", fullLocalPath, err)
					}
				}
			}
		}()
	}

	// 遍历本地文件
	err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过隐藏文件和目录
		if strings.HasPrefix(filepath.Base(path), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			// 计算相对路径
			relPath, err := filepath.Rel(localPath, path)
			if err != nil {
				return err
			}

			// 转换 Windows 路径分隔符
			relPath = strings.ReplaceAll(relPath, "\\", "/")

			// 发送任务
			jobCh <- relPath
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("遍历本地目录失败: %w", err)
	}

	// 关闭任务通道并等待所有工作线程完成
	close(jobCh)
	wg.Wait()

	// 如果需要，删除远程不存在的文件
	if c.Bool("delete") {
		for remotePath, info := range remoteFiles {
			// 使用互斥锁保护 map 读取
			processedMutex.Lock()
			processed := processedFiles[remotePath]
			processedMutex.Unlock()

			if !processed {
				fmt.Printf("删除: %s\n", remotePath)
				err := client.RemoveObject(ctx, session.BucketName, info.Key, minio.RemoveObjectOptions{})
				if err != nil {
					fmt.Fprintf(os.Stderr, "删除远程文件失败 '%s': %v\n", remotePath, err)
				}
			}
		}
	}

	fmt.Printf("同步完成: %s -> %s\n", localPath, formattedPath)
	return nil
}

// 生成认证字符串操作
func authAction(c *cli.Context) error {
	if c.NArg() < 3 {
		return fmt.Errorf("需要提供 endpoint、accessKey、secretKey 和 bucketName")
	}

	endpoint := c.Args().Get(0)
	accessKey := c.Args().Get(1)
	secretKey := c.Args().Get(2)
	bucketName := c.Args().Get(3)

	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	authString := fmt.Sprintf("%s:%s:%s:%s", endpoint, accessKey, secretKey, bucketName)
	fmt.Println(authString)

	return nil
}

// 用于在外部命令中使用 auth 字符串
func getSessionFromAuth(c *cli.Context) (*Session, *minio.Client, error) {
	authStr := c.String("auth")
	if authStr == "" {
		// 使用当前会话
		manager, err := initSessionManager()
		if err != nil {
			return nil, nil, err
		}

		session, err := manager.CurrentSession()
		if err != nil {
			return nil, nil, err
		}

		client, err := manager.GetClient()
		if err != nil {
			return nil, nil, err
		}

		return session, client, nil
	}

	// 使用 auth 字符串
	session, err := ParseAuthString(authStr)
	if err != nil {
		return nil, nil, err
	}

	// 创建客户端
	client, err := minio.New(session.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(session.AccessKey, session.SecretKey, ""),
		Secure: strings.HasPrefix(session.Endpoint, "https://"),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("创建客户端失败: %w", err)
	}

	return session, client, nil
}
