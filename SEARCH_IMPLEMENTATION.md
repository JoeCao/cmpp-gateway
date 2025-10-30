# 搜索功能实现文档

## 概述

本次重构为CMPP Gateway添加了完整的消息历史记录搜索功能，支持下发记录和上行记录的多条件搜索，并与现有的分页系统完美集成。

## 实现背景

- **问题**: 原有的消息历史记录页面只支持按时间分页浏览，无法根据特定条件筛选消息
- **需求**: 用户需要能够按照接收号码、发送号码、消息内容和发送状态等条件快速查找特定消息
- **技术要求**: 搜索功能需要同时支持BoltDB和Redis两种缓存后端

## 功能特性

### 🎯 搜索条件

#### 下发记录搜索 (`/list_message`)
- **接收号码** (`dest`): 支持模糊匹配手机号码
- **发送状态** (`status`): 支持筛选成功(0)和失败(1)的消息
- **内容关键词** (`content`): 支持模糊匹配短信内容

#### 上行记录搜索 (`/list_mo`)
- **发送号码** (`src`): 支持模糊匹配手机号码
- **接收号码** (`dest`): 支持模糊匹配服务号码
- **内容关键词** (`content`): 支持模糊匹配短信内容

### 🔧 技术特性
- **不区分大小写搜索**: 所有文本搜索都转换为小写进行比较
- **复合条件搜索**: 支持多个搜索条件同时生效
- **URL参数保持**: 分页导航时保持搜索条件
- **表单状态保持**: 搜索表单自动填充当前搜索条件
- **清除搜索**: 一键清除所有搜索条件

## 技术实现

### 1. 缓存接口扩展

#### CacheInterface 接口更新
```go
type CacheInterface interface {
    // 原有方法...
    SetWaitCache(key uint32, message SmsMes) error
    GetWaitCache(key uint32) (SmsMes, error)
    AddSubmits(mes *SmsMes) error
    AddMoList(mes *SmsMes) error
    Length(listName string) int
    GetStats() map[string]int
    GetList(listName string, start, end int) *[]SmsMes

    // 新增搜索方法
    SearchList(listName string, filters map[string]string, start, end int) *[]SmsMes
    GetSearchCount(listName string, filters map[string]string) int
}
```

### 2. Redis 实现搜索功能

#### 核心搜索逻辑
```go
// SearchList 在Redis中搜索消息列表
func (c *Cache) SearchList(listName string, filters map[string]string, start, end int) *[]SmsMes {
    // 1. 获取所有数据进行内存过滤（Redis不支持复杂搜索）
    // 2. 根据过滤条件筛选消息
    // 3. 应用分页逻辑
    // 4. 返回结果
}
```

#### 过滤条件实现
```go
func (c *Cache) matchFilters(mes *SmsMes, filters map[string]string, listName string) bool {
    // 通用过滤：内容关键词
    if content, ok := filters["content"]; ok && content != "" {
        if !contains(mes.Content, content) {
            return false
        }
    }

    // 下发消息特定过滤：接收号码、发送状态
    // 上行消息特定过滤：发送号码、接收号码
}
```

### 3. BoltDB 实现搜索功能

#### 搜索实现
```go
// SearchList 在BoltDB中搜索消息列表
func (c *BoltCache) SearchList(listName string, filters map[string]string, start, end int) *[]SmsMes {
    // 1. 遍历数据库中的所有记录
    // 2. 应用过滤条件
    // 3. 收集匹配的消息
    // 4. 应用分页逻辑
}
```

### 4. HTTP 处理器更新

#### 搜索参数处理
```go
func listMessage(w http.ResponseWriter, r *http.Request, listName string, activePage string) {
    // 解析搜索参数
    filters := make(map[string]string)
    if listName == "list_message" {
        if dest := r.Form.Get("dest"); dest != "" {
            filters["dest"] = dest
        }
        if status := r.Form.Get("status"); status != "" {
            filters["status"] = status
        }
    } else if listName == "list_mo" {
        if src := r.Form.Get("src"); src != "" {
            filters["src"] = src
        }
        if dest := r.Form.Get("dest"); dest != "" {
            filters["dest"] = dest
        }
    }
    if content := r.Form.Get("content"); content != "" {
        filters["content"] = content
    }

    // 根据是否有搜索条件选择不同的处理方式
    var count int
    var v *[]SmsMes
    if len(filters) > 0 {
        count = SCache.GetSearchCount(listName, filters)
        page := pages.NewPage(c_page, pageSize, count)
        v = SCache.SearchList(listName, filters, page.StartRow, page.EndRow)
    } else {
        count = SCache.Length(listName)
        page := pages.NewPage(c_page, pageSize, count)
        v = SCache.GetList(listName, page.StartRow, page.EndRow)
    }
}
```

### 5. 前端模板更新

#### 搜索表单增强
- **表单字段**: 添加了相应的name属性以支持表单提交
- **值保持**: 使用模板变量保持当前搜索值
- **清除按钮**: 提供一键清除搜索功能

#### URL构建模板函数
```go
"buildPageURL": func(page int, filters map[string]string) string {
    params := make([]string, 0, len(filters)+1)
    params = append(params, fmt.Sprintf("page=%d", page))

    for key, value := range filters {
        if value != "" {
            params = append(params, fmt.Sprintf("%s=%s", key, value))
        }
    }

    return "?" + strings.Join(params, "&")
}
```

#### JavaScript 搜索逻辑
```javascript
// 表单提交处理
document.getElementById('filter-form').addEventListener('submit', function(e) {
    e.preventDefault();

    const formData = new FormData(this);
    const params = new URLSearchParams();

    // 构建搜索参数
    for (const [key, value] of formData.entries()) {
        if (value.trim() !== '') {
            params.append(key, value);
        }
    }

    // 重置到第一页并跳转
    params.set('page', '1');
    window.location.href = '?' + params.toString();
});

// 清除搜索
function clearSearch() {
    window.location.href = '?';
}
```

## 使用示例

### 1. 基本搜索
```
# 搜索接收号码为13800138000的所有消息
http://localhost:8000/list_message?dest=13800138000

# 搜索包含"重要"关键词的所有消息
http://localhost:8000/list_message?content=重要

# 搜索失败的消息
http://localhost:8000/list_message?status=1
```

### 2. 复合搜索
```
# 搜索接收号码为13800138000且包含"测试"的消息
http://localhost:8000/list_message?dest=13800138000&content=测试

# 搜索发送号码为13900139000且包含"你好"的上行消息
http://localhost:8000/list_mo?src=13900139000&content=你好
```

### 3. 分页搜索
```
# 搜索结果的第二页
http://localhost:8000/list_message?content=测试&page=2
```

## 性能考虑

### Redis 后端
- **策略**: 获取所有数据到内存进行过滤
- **优化**: 对于大量数据，可以考虑添加索引或使用Redis的搜索模块
- **当前限制**: 适合中小规模数据集

### BoltDB 后端
- **策略**: 遍历数据库进行过滤
- **优化**: 利用BoltDB的高效遍历性能
- **优势**: 适合本地部署，无需额外依赖

### 通用优化
- **分页限制**: 只加载当前页需要的数据
- **缓存友好**: 搜索结果可以缓存以提高性能
- **索引建议**: 未来可以添加专门的数据结构支持高效搜索

## 测试验证

### 功能测试
✅ 按接收号码搜索正常工作
✅ 按发送号码搜索正常工作
✅ 按内容关键词搜索正常工作
✅ 按发送状态搜索正常工作
✅ 复合条件搜索正常工作
✅ 搜索条件在表单中正确保持
✅ 分页链接正确保持搜索参数
✅ 清除搜索功能正常

### 兼容性测试
✅ BoltDB后端搜索功能正常
✅ Redis后端搜索功能正常
✅ 无搜索参数时正常显示全部记录

## 后续优化建议

### 1. 性能优化
- **数据索引**: 为常用搜索字段添加索引
- **搜索缓存**: 缓存热门搜索条件的结果
- **异步搜索**: 对于大量数据，考虑异步搜索

### 2. 功能扩展
- **日期范围搜索**: 支持按时间段筛选
- **正则表达式搜索**: 支持更复杂的文本匹配
- **搜索历史**: 保存用户的搜索历史

### 3. 用户体验
- **搜索建议**: 提供搜索关键词自动完成
- **搜索高亮**: 在结果中高亮匹配的关键词
- **搜索统计**: 显示搜索结果数量和耗时

## 部署说明

### 配置更新
无需额外配置，搜索功能会自动适配现有的缓存后端配置。

### 路由更新
新增了`/submit`路由以保持API一致性：
```go
http.HandleFunc("/submit", handler)  // 主要API端点
http.HandleFunc("/send", handler)   // 向后兼容
```

### 数据库迁移
现有数据完全兼容，无需数据迁移。

## 总结

本次搜索功能实现为CMPP Gateway增加了强大的消息检索能力，在保持系统性能的同时提供了灵活的搜索选项。通过精心设计的接口和实现，搜索功能能够无缝集成到现有系统中，为用户提供更好的使用体验。

### 主要成就
- 🎯 **完整搜索功能**: 支持多条件复合搜索
- 🔄 **无缝集成**: 与现有分页系统完美结合
- 💾 **双后端支持**: 同时支持BoltDB和Redis
- 📱 **用户友好**: 直观的搜索界面和交互
- 🚀 **高性能**: 优化的搜索算法和数据访问

这次重构为CMPP Gateway的搜索功能奠定了坚实的基础，为未来的功能扩展提供了良好的架构支持。