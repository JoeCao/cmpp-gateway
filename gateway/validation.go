package gateway

import (
	"fmt"
	"regexp"
	"strconv"
	"unicode/utf8"
)

// 验证规则常量
const (
	// 短信内容最大长度（字符数，非字节数）
	// 70字符 ≈ 1条短信（GB18030 编码下）
	MaxSMSContentLength = 500

	// 扩展码最大长度
	MaxExtCodeLength = 6

	// 手机号长度
	PhoneNumberLength = 11
)

var (
	// 手机号正则: 1[3-9]\d{9}
	// 支持 13x, 14x, 15x, 16x, 17x, 18x, 19x 号段
	phoneRegex = regexp.MustCompile(`^1[3-9]\d{9}$`)

	// 扩展码正则: 纯数字，1-6位
	extCodeRegex = regexp.MustCompile(`^\d{1,6}$`)
)

// ValidationError 表示参数验证错误
type ValidationError struct {
	Field   string // 错误字段名
	Message string // 错误消息
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateSubmitParams 验证短信提交参数
//
// 参数:
//   - src: 扩展码（可选）
//   - dest: 目标手机号（必填）
//   - content: 短信内容（必填）
//
// 返回:
//   - normalizedContent: 与输入一致的内容（验证仅负责校验，不修改正文）
//   - error: 验证失败时返回 ValidationError
func ValidateSubmitParams(src, dest, content string) (normalizedContent string, err error) {
	// 1. 验证目标号码（必填）
	if dest == "" {
		return "", &ValidationError{
			Field:   "dest",
			Message: "目标手机号不能为空",
		}
	}

	if !phoneRegex.MatchString(dest) {
		return "", &ValidationError{
			Field:   "dest",
			Message: fmt.Sprintf("无效的手机号: %s（格式应为 1[3-9]xxxxxxxxx）", dest),
		}
	}

	// 2. 验证扩展码（可选，但如果提供则必须符合格式）
	if src != "" && !extCodeRegex.MatchString(src) {
		return "", &ValidationError{
			Field:   "src",
			Message: fmt.Sprintf("无效的扩展码: %s（仅支持1-6位数字）", src),
		}
	}

	// 3. 验证短信内容（必填）
	if content == "" {
		return "", &ValidationError{
			Field:   "cont",
			Message: "短信内容不能为空",
		}
	}

	// 4. 验证内容长度（使用 rune 计数，不是字节数）
	contentLength := utf8.RuneCountInString(content)
	if contentLength > MaxSMSContentLength {
		return "", &ValidationError{
			Field:   "cont",
			Message: fmt.Sprintf("短信内容过长（当前 %d 字符，最大 %d 字符）", contentLength, MaxSMSContentLength),
		}
	}

	// 5. 验证通过后返回原始内容；HTML 转义交由模板渲染层处理，避免重复编码
	return content, nil
}

// ValidateSearchParams 验证搜索参数
//
// 参数:
//   - dest: 目标手机号（可选）
//   - src: 源号码或扩展码（可选）
//   - content: 内容关键词（可选）
//
// 返回:
//   - error: 验证失败时返回 ValidationError
func ValidateSearchParams(dest, src, content string) error {
	// 如果提供了目标号码，验证格式
	if dest != "" && !phoneRegex.MatchString(dest) {
		return &ValidationError{
			Field:   "dest",
			Message: fmt.Sprintf("无效的手机号: %s", dest),
		}
	}

	// 如果提供了源号码，验证格式（可能是扩展码或完整号码）
	if src != "" {
		// 尝试匹配扩展码格式
		if !extCodeRegex.MatchString(src) {
			// 尝试匹配完整手机号格式
			if !phoneRegex.MatchString(src) {
				return &ValidationError{
					Field:   "src",
					Message: fmt.Sprintf("无效的源号码: %s（应为1-6位数字扩展码或11位手机号）", src),
				}
			}
		}
	}

	// 内容关键词不需要严格验证，但限制长度防止滥用
	if content != "" && utf8.RuneCountInString(content) > 100 {
		return &ValidationError{
			Field:   "content",
			Message: "搜索关键词过长（最大100字符）",
		}
	}

	return nil
}

// ValidatePageParam 验证分页参数
//
// 参数:
//   - page: 页码字符串
//
// 返回:
//   - pageNum: 有效的页码（>=1）
//   - error: 验证失败时返回 ValidationError
func ValidatePageParam(page string) (pageNum int, err error) {
	if page == "" {
		return 1, nil // 默认第一页
	}

	// 尝试解析为整数
	num, err := strconv.Atoi(page)
	if err != nil {
		return 0, &ValidationError{
			Field:   "page",
			Message: fmt.Sprintf("无效的页码: %s（必须为正整数）", page),
		}
	}

	// 验证范围
	if num < 1 {
		return 0, &ValidationError{
			Field:   "page",
			Message: fmt.Sprintf("无效的页码: %d（必须 >= 1）", num),
		}
	}

	// 防止过大的页码（可能导致性能问题）
	if num > 10000 {
		return 0, &ValidationError{
			Field:   "page",
			Message: fmt.Sprintf("页码过大: %d（最大10000）", num),
		}
	}

	return num, nil
}
