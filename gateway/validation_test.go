package gateway

import (
	"strings"
	"testing"
)

// ========== ValidateSubmitParams 测试 ==========

func TestValidateSubmitParams_Success(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		dest    string
		content string
		want    string // 期望的返回内容（保持原文）
	}{
		{
			name:    "正常提交-无扩展码",
			src:     "",
			dest:    "13800138000",
			content: "你好，测试短信",
			want:    "你好，测试短信",
		},
		{
			name:    "正常提交-有扩展码",
			src:     "123",
			dest:    "13912345678",
			content: "验证码: 123456",
			want:    "验证码: 123456",
		},
		{
			name:    "HTML转义测试",
			src:     "",
			dest:    "18600001234",
			content: "<script>alert('xss')</script>",
			want:    "<script>alert('xss')</script>",
		},
		{
			name:    "特殊字符",
			src:     "1",
			dest:    "15900000000",
			content: "订单号: #12345 & 金额: ¥100",
			want:    "订单号: #12345 & 金额: ¥100",
		},
		{
			name:    "6位扩展码",
			src:     "123456",
			dest:    "13700000000",
			content: "最大长度扩展码",
			want:    "最大长度扩展码",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := ValidateSubmitParams(tt.src, tt.dest, tt.content)
			if err != nil {
				t.Errorf("ValidateSubmitParams() error = %v, want nil", err)
				return
			}
			if content != tt.want {
				t.Errorf("ValidateSubmitParams() content = %q, want %q", content, tt.want)
			}
		})
	}
}

func TestValidateSubmitParams_InvalidDest(t *testing.T) {
	tests := []struct {
		name      string
		dest      string
		wantField string
	}{
		{
			name:      "空手机号",
			dest:      "",
			wantField: "dest",
		},
		{
			name:      "手机号太短",
			dest:      "138001380",
			wantField: "dest",
		},
		{
			name:      "手机号太长",
			dest:      "138001380000",
			wantField: "dest",
		},
		{
			name:      "无效号段",
			dest:      "12800138000",
			wantField: "dest",
		},
		{
			name:      "包含字母",
			dest:      "1380013800a",
			wantField: "dest",
		},
		{
			name:      "包含特殊字符",
			dest:      "138-0013-8000",
			wantField: "dest",
		},
		{
			name:      "固定电话",
			dest:      "02112345678",
			wantField: "dest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateSubmitParams("", tt.dest, "测试内容")
			if err == nil {
				t.Errorf("ValidateSubmitParams() error = nil, want ValidationError")
				return
			}
			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Errorf("ValidateSubmitParams() error type = %T, want *ValidationError", err)
				return
			}
			if validationErr.Field != tt.wantField {
				t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, tt.wantField)
			}
		})
	}
}

func TestValidateSubmitParams_InvalidSrc(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantField string
	}{
		{
			name:      "扩展码包含字母",
			src:       "12a",
			wantField: "src",
		},
		{
			name:      "扩展码过长",
			src:       "1234567",
			wantField: "src",
		},
		{
			name:      "扩展码包含特殊字符",
			src:       "123-",
			wantField: "src",
		},
		{
			name:      "负数扩展码",
			src:       "-123",
			wantField: "src",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateSubmitParams(tt.src, "13800138000", "测试内容")
			if err == nil {
				t.Errorf("ValidateSubmitParams() error = nil, want ValidationError")
				return
			}
			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Errorf("ValidateSubmitParams() error type = %T, want *ValidationError", err)
				return
			}
			if validationErr.Field != tt.wantField {
				t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, tt.wantField)
			}
		})
	}
}

func TestValidateSubmitParams_InvalidContent(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantField string
	}{
		{
			name:      "空内容",
			content:   "",
			wantField: "cont",
		},
		{
			name:      "超长内容",
			content:   strings.Repeat("测试", 251), // 502 字符
			wantField: "cont",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateSubmitParams("", "13800138000", tt.content)
			if err == nil {
				t.Errorf("ValidateSubmitParams() error = nil, want ValidationError")
				return
			}
			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Errorf("ValidateSubmitParams() error type = %T, want *ValidationError", err)
				return
			}
			if validationErr.Field != tt.wantField {
				t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, tt.wantField)
			}
		})
	}
}

func TestValidateSubmitParams_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		dest        string
		content     string
		wantErr     bool
		wantErrType string // "dest", "cont", etc.
	}{
		{
			name:    "最大长度内容-边界值",
			src:     "",
			dest:    "13800138000",
			content: strings.Repeat("测", 500), // 恰好 500 字符
			wantErr: false,
		},
		{
			name:        "超出最大长度1字符",
			src:         "",
			dest:        "13800138000",
			content:     strings.Repeat("测", 501), // 501 字符
			wantErr:     true,
			wantErrType: "cont",
		},
		{
			name:    "各号段测试-13x",
			dest:    "13000000000",
			content: "test",
			wantErr: false,
		},
		{
			name:    "各号段测试-14x",
			dest:    "14000000000",
			content: "test",
			wantErr: false,
		},
		{
			name:    "各号段测试-19x",
			dest:    "19900000000",
			content: "test",
			wantErr: false,
		},
		{
			name:    "单字符扩展码",
			src:     "1",
			dest:    "13800138000",
			content: "test",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateSubmitParams(tt.src, tt.dest, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSubmitParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				validationErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("error type = %T, want *ValidationError", err)
					return
				}
				if tt.wantErrType != "" && validationErr.Field != tt.wantErrType {
					t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, tt.wantErrType)
				}
			}
		})
	}
}

// ========== ValidateSearchParams 测试 ==========

func TestValidateSearchParams_Success(t *testing.T) {
	tests := []struct {
		name    string
		dest    string
		src     string
		content string
	}{
		{
			name:    "全部为空",
			dest:    "",
			src:     "",
			content: "",
		},
		{
			name:    "只搜索目标号码",
			dest:    "13800138000",
			src:     "",
			content: "",
		},
		{
			name:    "只搜索扩展码",
			dest:    "",
			src:     "123",
			content: "",
		},
		{
			name:    "搜索完整手机号",
			dest:    "",
			src:     "13900000000",
			content: "",
		},
		{
			name:    "只搜索内容",
			dest:    "",
			src:     "",
			content: "关键词",
		},
		{
			name:    "组合搜索",
			dest:    "13800138000",
			src:     "123",
			content: "验证码",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSearchParams(tt.dest, tt.src, tt.content)
			if err != nil {
				t.Errorf("ValidateSearchParams() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateSearchParams_InvalidParams(t *testing.T) {
	tests := []struct {
		name      string
		dest      string
		src       string
		content   string
		wantField string
	}{
		{
			name:      "无效目标号码",
			dest:      "12345",
			src:       "",
			content:   "",
			wantField: "dest",
		},
		{
			name:      "无效源号码-非数字",
			dest:      "",
			src:       "abc",
			content:   "",
			wantField: "src",
		},
		{
			name:      "无效源号码-扩展码过长",
			dest:      "",
			src:       "1234567",
			content:   "",
			wantField: "src",
		},
		{
			name:      "搜索关键词过长",
			dest:      "",
			src:       "",
			content:   strings.Repeat("测", 101), // 101 字符
			wantField: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSearchParams(tt.dest, tt.src, tt.content)
			if err == nil {
				t.Errorf("ValidateSearchParams() error = nil, want ValidationError")
				return
			}
			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Errorf("error type = %T, want *ValidationError", err)
				return
			}
			if validationErr.Field != tt.wantField {
				t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, tt.wantField)
			}
		})
	}
}

// ========== ValidatePageParam 测试 ==========

func TestValidatePageParam_Success(t *testing.T) {
	tests := []struct {
		name     string
		page     string
		wantPage int
	}{
		{
			name:     "空参数-默认第1页",
			page:     "",
			wantPage: 1,
		},
		{
			name:     "第1页",
			page:     "1",
			wantPage: 1,
		},
		{
			name:     "第100页",
			page:     "100",
			wantPage: 100,
		},
		{
			name:     "最大页码",
			page:     "10000",
			wantPage: 10000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pageNum, err := ValidatePageParam(tt.page)
			if err != nil {
				t.Errorf("ValidatePageParam() error = %v, want nil", err)
				return
			}
			if pageNum != tt.wantPage {
				t.Errorf("ValidatePageParam() pageNum = %d, want %d", pageNum, tt.wantPage)
			}
		})
	}
}

func TestValidatePageParam_InvalidParams(t *testing.T) {
	tests := []struct {
		name      string
		page      string
		wantField string
	}{
		{
			name:      "负数页码",
			page:      "-1",
			wantField: "page",
		},
		{
			name:      "零页码",
			page:      "0",
			wantField: "page",
		},
		{
			name:      "非数字",
			page:      "abc",
			wantField: "page",
		},
		{
			name:      "浮点数",
			page:      "1.5",
			wantField: "page",
		},
		{
			name:      "超大页码",
			page:      "10001",
			wantField: "page",
		},
		{
			name:      "包含特殊字符",
			page:      "1; DROP TABLE",
			wantField: "page",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidatePageParam(tt.page)
			if err == nil {
				t.Errorf("ValidatePageParam() error = nil, want ValidationError")
				return
			}
			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Errorf("error type = %T, want *ValidationError", err)
				return
			}
			if validationErr.Field != tt.wantField {
				t.Errorf("ValidationError.Field = %q, want %q", validationErr.Field, tt.wantField)
			}
		})
	}
}

// ========== 性能基准测试 ==========

func BenchmarkValidateSubmitParams_Success(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidateSubmitParams("123", "13800138000", "测试短信内容")
	}
}

func BenchmarkValidateSubmitParams_RawHTML(b *testing.B) {
	content := "<script>alert('xss')</script>"
	for i := 0; i < b.N; i++ {
		ValidateSubmitParams("", "13800138000", content)
	}
}

func BenchmarkValidateSearchParams(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidateSearchParams("13800138000", "123", "关键词")
	}
}

func BenchmarkValidatePageParam(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ValidatePageParam("42")
	}
}
