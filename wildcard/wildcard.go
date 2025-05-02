package wildcard

// Match 检查文本是否匹配通配符模式
func Match(pattern, text string) bool {
	// 检查模式为空或只由星号组成
	if len(pattern) == 0 {
		return len(text) == 0
	}

	// 如果只包含星号
	if pattern == "*" {
		return true
	}

	// 使用动态规划解决
	dp := make([][]bool, len(text)+1)
	for i := range dp {
		dp[i] = make([]bool, len(pattern)+1)
	}

	// 空字符串匹配空模式
	dp[0][0] = true

	// 处理只有星号的情况
	for j := 1; j <= len(pattern); j++ {
		if pattern[j-1] == '*' {
			dp[0][j] = dp[0][j-1]
		}
	}

	// 填充dp表
	for i := 1; i <= len(text); i++ {
		for j := 1; j <= len(pattern); j++ {
			if pattern[j-1] == '*' {
				dp[i][j] = dp[i][j-1] || dp[i-1][j]
			} else if pattern[j-1] == '?' || pattern[j-1] == text[i-1] {
				dp[i][j] = dp[i-1][j-1]
			}
		}
	}

	return dp[len(text)][len(pattern)]
}
