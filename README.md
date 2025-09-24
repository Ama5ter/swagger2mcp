# swagger2mcp

一个将现有 Swagger/OpenAPI 定义转换为 Model Context Protocol (MCP) 工具脚手架的命令行生成器。CLI 可读取本地文件或 HTTP URL 中的 OpenAPI v3 与 Swagger v2 规格，对其进行规范化处理，并生成 Go、Node (npm) 或 Python 的 MCP 工具初始项目。

## 亮点
- 自动将 Swagger v2 转换为 OpenAPI v3，并提供清晰的验证与错误提示。
- 支持远程规格抓取，具备重试和退避策略；加载本地文件时可按需启用外部引用。
- 可生成带标签过滤的 Go、npm、Python MCP 工具骨架，带有贴心的默认结构。
- 支持预览模式、覆盖保护、自定义工具/模块命名等高级选项。
- 提供 `init` 命令自动写出带注释的配置文件，详细说明每个可用选项。

## 安装
- 使用 Go 1.23 或更新版本：
  ```bash
  go install github.com/mark3labs/swagger2mcp/cmd/swagger2mcp@latest
  ```
- 从源码构建：
  ```bash
  git clone https://github.com/mark3labs/swagger2mcp.git
  cd swagger2mcp
  make build            # 生成 ./bin/swagger2mcp
  ```

## 使用
执行 `swagger2mcp --help` 查看顶层命令。全局标志：
- `--config`, `-c`：从 YAML/JSON 配置文件加载默认值（命令行标志优先生效）。
- `--verbose`, `-v`：开启详细日志输出。

### Generate
从规格文件生成 MCP 工具项目：
```bash
swagger2mcp generate \
  --input swagger.yaml \
  --lang go \
  --out ./out-go \
  --include-tags public --exclude-tags internal \
  --tool-name petstore \
  --package-name github.com/example/petstore
```
关键标志说明：
- `--input` *(必填)*：Swagger/OpenAPI 文档的路径或 URL。
- `--lang`：选择 `go`（默认）、`npm` 或 `python`。
- `--out`：输出目录（未提供时默认使用推导出的工具名）。
- `--tool-name`：覆盖生成的工具名称；会被标准化为小写加短横线。
- `--package-name`：Go 模块名或 npm/Python 包名。
- `--include-tags` / `--exclude-tags`：按 OpenAPI 标签筛选操作（会自动去重和去空白）。
- `--dry-run`：仅显示将写入的文件列表，而不修改文件系统。
- `--force`：允许覆盖已存在的输出目录。

当校验失败时（如未知语言、标签筛选冲突、权限问题），生成器会返回友好的提示信息。

### Init
生成包含注释的配置模板，帮助理解所有可用选项：
```bash
swagger2mcp init --out swagger2mcp.yaml
```
如需覆盖已存在文件，可添加 `--force`。示例配置：
```yaml
# swagger2mcp configuration (YAML)
# input: ./openapi.yaml
# lang: go
# out: ./out
# includeTags: [public, read]
# excludeTags: [internal]
# toolName: api-docs
# packageName: example.com/mytool
# dryRun: false
# force: false
# verbose: false
```
结合 `--config` 与 `generate` 命令使用，可集中管理默认参数。

## 示例数据
仓库内包含一个简易 `swagger.yaml` 可供试验：
```bash
swagger2mcp generate --input swagger.yaml --lang npm --out ./tmp/out-npm --force
```

## 开发
- 运行单元测试：`make test`
- 使用内置样例进行端到端测试：`make e2e`
- 启用在线端到端测试（需访问远程规格）：`make e2e-online`
- 代码格式化：`make fmt`
- 重新生成样例项目：`make generate-go-sample` / `make generate-npm-sample`

## 故障排查
- 若生成时出现权限或只读错误，说明目标目录不可写，请更换 `--out` 或在确认后使用 `--force`。
- 远程抓取失败时会自动重试并采用指数退避；可开启 `--verbose` 查看请求详情。
- 如需加载包含 `file://` 引用的多文件本地规格，请从本地文件路径启动以自动允许该类引用。

## 许可
请参阅仓库中的许可文件。
