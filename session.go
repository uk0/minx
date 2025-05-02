package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Session 代表一个 Minio 会话
type Session struct {
	Endpoint    string `json:"endpoint"`
	AccessKey   string `json:"access_key"`
	SecretKey   string `json:"secret_key"`
	BucketName  string `json:"bucket_name"`
	CurrentPath string `json:"current_path"`
}

// SessionManager 管理所有会话
type SessionManager struct {
	Sessions      map[string]Session `json:"sessions"`
	CurrentName   string             `json:"current_name"`
	ConfigPath    string             `json:"-"`
	currentClient *minio.Client      `json:"-"`
}

var manager *SessionManager

// 初始化会话管理器
func initSessionManager() (*SessionManager, error) {
	if manager != nil {
		return manager, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("无法获取用户主目录: %w", err)
	}

	configDir := filepath.Join(homeDir, ".minx")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建配置目录: %w", err)
	}

	configPath := filepath.Join(configDir, "config.json")

	manager = &SessionManager{
		Sessions:   make(map[string]Session),
		ConfigPath: configPath,
	}

	// 尝试加载现有配置
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("无法读取配置文件: %w", err)
		}

		if err := json.Unmarshal(data, manager); err != nil {
			return nil, fmt.Errorf("无法解析配置文件: %w", err)
		}
	}

	return manager, nil
}

// 保存会话管理器配置
func (m *SessionManager) Save() error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("无法序列化配置: %w", err)
	}

	if err := os.WriteFile(m.ConfigPath, data, 0644); err != nil {
		return fmt.Errorf("无法写入配置文件: %w", err)
	}

	return nil
}

// 添加新会话
func (m *SessionManager) AddSession(name string, session Session) error {
	m.Sessions[name] = session
	if m.CurrentName == "" {
		m.CurrentName = name
	}
	return m.Save()
}

// 删除会话
func (m *SessionManager) RemoveSession(name string) error {
	if _, exists := m.Sessions[name]; !exists {
		return fmt.Errorf("会话 '%s' 不存在", name)
	}

	delete(m.Sessions, name)

	// 如果删除的是当前会话，需要重新选择一个会话
	if m.CurrentName == name {
		if len(m.Sessions) > 0 {
			for k := range m.Sessions {
				m.CurrentName = k
				break
			}
		} else {
			m.CurrentName = ""
		}
	}

	return m.Save()
}

// 切换当前会话
func (m *SessionManager) SwitchSession(name string) error {
	if _, exists := m.Sessions[name]; !exists {
		return fmt.Errorf("会话 '%s' 不存在", name)
	}

	m.CurrentName = name
	m.currentClient = nil // 清除缓存的客户端
	return m.Save()
}

// 获取当前会话
func (m *SessionManager) CurrentSession() (*Session, error) {
	if m.CurrentName == "" {
		return nil, errors.New("没有活动会话，请先使用 login 命令登录")
	}

	session, exists := m.Sessions[m.CurrentName]
	if !exists {
		return nil, fmt.Errorf("当前会话 '%s' 不存在", m.CurrentName)
	}

	return &session, nil
}

// 获取当前 Minio 客户端
func (m *SessionManager) GetClient() (*minio.Client, error) {
	if m.currentClient != nil {
		return m.currentClient, nil
	}

	session, err := m.CurrentSession()
	if err != nil {
		return nil, err
	}

	// 处理 endpoint
	endpoint := session.Endpoint
	secure := true

	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
		secure = false
	} else if strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	// 创建 Minio 客户端
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(session.AccessKey, session.SecretKey, ""),
		Secure: secure,
	})

	if err != nil {
		return nil, fmt.Errorf("创建 Minio 客户端失败: %w", err)
	}

	m.currentClient = client
	return client, nil
}

// 更新当前路径
func (m *SessionManager) UpdateCurrentPath(path string) error {
	if m.CurrentName == "" {
		return errors.New("没有活动会话，请先使用 login 命令登录")
	}

	session := m.Sessions[m.CurrentName]
	session.CurrentPath = path
	m.Sessions[m.CurrentName] = session
	return m.Save()
}

// 格式化路径
func (m *SessionManager) FormatPath(path string) (string, error) {
	session, err := m.CurrentSession()
	if err != nil {
		return "", err
	}

	if path == "" {
		return session.CurrentPath, nil
	}

	if path[0] == '/' {
		// 绝对路径
		return path, nil
	} else {
		// 相对路径
		if session.CurrentPath == "/" {
			return "/" + path, nil
		}
		return session.CurrentPath + "/" + path, nil
	}
}

// 解析认证字符串
func ParseAuthString(authStr string) (*Session, error) {
	parts := strings.Split(authStr, ":")
	if len(parts) != 4 {
		return nil, errors.New("无效的认证字符串格式，应为 endpoint:accessKey:secretKey:bucketName")
	}

	return &Session{
		Endpoint:    parts[0],
		AccessKey:   parts[1],
		SecretKey:   parts[2],
		BucketName:  parts[3],
		CurrentPath: "/",
	}, nil
}
