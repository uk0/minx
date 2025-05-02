package main

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// ProgressBar 表示进度条
type ProgressBar struct {
	Total     int64
	Current   int64
	Width     int
	FileName  string
	StartTime time.Time
}

// Update 更新进度条
func (p *ProgressBar) Update(n int64) {
	atomic.AddInt64(&p.Current, n)
	p.Draw()
}

// Draw 绘制进度条
func (p *ProgressBar) Draw() {
	current := atomic.LoadInt64(&p.Current)

	percent := float64(current) / float64(p.Total) * 100
	if percent > 100 {
		percent = 100
	}

	// 计算速度
	elapsed := time.Since(p.StartTime).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(current) / elapsed
	}

	// 格式化速度
	var speedStr string
	if speed < 1024 {
		speedStr = fmt.Sprintf("%.2f B/s", speed)
	} else if speed < 1024*1024 {
		speedStr = fmt.Sprintf("%.2f KB/s", speed/1024)
	} else if speed < 1024*1024*1024 {
		speedStr = fmt.Sprintf("%.2f MB/s", speed/(1024*1024))
	} else {
		speedStr = fmt.Sprintf("%.2f GB/s", speed/(1024*1024*1024))
	}

	// 计算剩余时间
	var etaStr string
	if speed > 0 {
		etaSec := float64(p.Total-current) / speed
		if etaSec < 60 {
			etaStr = fmt.Sprintf("%.0fs", etaSec)
		} else if etaSec < 3600 {
			etaStr = fmt.Sprintf("%dm%ds", int(etaSec)/60, int(etaSec)%60)
		} else {
			etaStr = fmt.Sprintf("%dh%dm%ds", int(etaSec)/3600, (int(etaSec)%3600)/60, int(etaSec)%60)
		}
	} else {
		etaStr = "计算中..."
	}

	// 绘制进度条
	fmt.Printf("\r%s: [", p.FileName)

	completed := int(float64(p.Width) * (float64(current) / float64(p.Total)))
	for i := 0; i < p.Width; i++ {
		if i < completed {
			fmt.Print("=")
		} else if i == completed {
			fmt.Print(">")
		} else {
			fmt.Print(" ")
		}
	}

	fmt.Printf("] %.2f%% %s %s/s ETA: %s",
		percent, formatSize(current), speedStr, etaStr)
}

// ProgressReader 是一个带有进度跟踪的包装读取器
type ProgressReader struct {
	reader   io.Reader
	progress *ProgressBar
}

// NewProgressReader 创建一个新的进度读取器
func NewProgressReader(reader io.Reader, progress *ProgressBar) *ProgressReader {
	return &ProgressReader{
		reader:   reader,
		progress: progress,
	}
}

// Read 实现 io.Reader 接口
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.progress.Update(int64(n))
	}
	return n, err
}
