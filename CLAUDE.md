# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 常用命令

### 构建
```bash
# 构建主程序
make build
# 或
go build -o bin/swagger2mcp ./cmd/swagger2mcp

# 运行程序
make run
# 或
go run ./cmd/swagger2mcp --help
```

### 测试
```bash
# 运行所有单元测试
make test
# 或
go test ./...

# 运行端到端测试
make e2e

# 运行在线端到端测试
make e2e-online

# 完整集成测试（测试生成的Go和NPM项目）
./test_mcp.sh

# 综合逻辑流程测试
./test_comprehensive.sh
```

### 代码质量
```bash
# 格式化代码
make fmt
# 或
go fmt ./...

# 整理模块依赖
make tidy
# 或
go mod tidy
```

### 生成示例项目
```bash
# 生成Go版本MCP工具
make generate-go-sample

# 生成NPM版本MCP工具
make generate-npm-sample

# 生成Python版本MCP工具
go run ./cmd/swagger2mcp generate --input testdata/swagger.yaml --lang python --out tmp/out-python --force
```

## 项目架构

这是一个Swagger/OpenAPI到MCP工具的代码生成器，将API规范转换为Model Context Protocol (MCP) 服务器。

### 核心组件

1. **CLI层** (`internal/cli/`)
   - `root.go`: Cobra根命令定义
   - `generate.go`: 生成命令实现
   - `init.go`: 初始化命令
   - 支持Go和NPM两种目标语言

2. **规范处理** (`internal/spec/`)
   - `loader.go`: Swagger/OpenAPI文档加载器
   - `model.go`: 内部数据模型定义
   - `normalize.go`: 规范标准化处理
   - `v2compat.go`: Swagger 2.0兼容性支持

3. **代码生成器** (`internal/emitter/`)
   - `goemitter/`: Go语言MCP服务器生成器
   - `npmemitter/`: NPM/TypeScript MCP服务器生成器
   - `pyemitter/`: Python MCP服务器生成器
   - 每个生成器都实现相同的接口，支持模板化代码生成

4. **入口点** (`cmd/swagger2mcp/`)
   - `main.go`: 程序主入口，调用CLI层

### 数据流

1. **输入**: Swagger/OpenAPI YAML/JSON文件
2. **解析**: 使用`getkin/kin-openapi`库解析API规范
3. **标准化**: 转换为内部ServiceModel数据结构
4. **生成**: 根据目标语言选择对应的emitter生成MCP服务器代码
5. **输出**: 完整的可执行MCP工具项目

### 生成的MCP工具功能

生成的MCP服务器提供五个核心工具：
- `listEndpoints`: 列出所有API端点概览
- `searchEndpoints`: 根据关键词搜索端点
- `getEndpointDetails`: 获取特定端点的详细信息
- `listSchemas`: 列出所有数据模型概览
- `getSchemaDetails`: 获取特定数据模型的详细信息

### 测试策略

- **单元测试**: 各模块独立测试，覆盖解析、标准化、生成逻辑
- **端到端测试**: 完整流程测试，从输入到输出验证
- **集成测试**: 验证生成的MCP服务器能正确响应JSON-RPC请求
- **逻辑流程测试**: 测试生成的服务器的实际业务逻辑

### 主要依赖

- `github.com/getkin/kin-openapi`: OpenAPI文档解析
- `github.com/spf13/cobra`: CLI框架
- Go标准库进行模板渲染和文件操作

## 开发指南

### 支持的目标语言

1. **Go** (`--lang go`): 生成标准Go项目，使用Gorilla Mux路由和内置HTTP服务器
2. **NPM/TypeScript** (`--lang npm`): 生成TypeScript项目，包含完整的npm包配置
3. **Python** (`--lang python`): 生成Python项目，使用现代Python工具链

### 配置文件支持

使用`--config`参数指定YAML配置文件，支持以下选项：
```yaml
input: swagger.yaml
lang: go
out: ./generated
tool_name: my-api-tool
package_name: my_api
include_tags:
  - users
  - orders
exclude_tags:
  - internal
force: true
dry_run: false
verbose: true
```

### 常见工作流

1. **开发新功能**: 修改相应的emitter包，运行单元测试验证
2. **添加新语言支持**: 创建新的emitter包，实现Emitter接口
3. **调试生成问题**: 使用`--dry-run`和`--verbose`标志检查生成计划
4. **验证输出**: 运行对应的测试脚本确保生成的代码正确工作