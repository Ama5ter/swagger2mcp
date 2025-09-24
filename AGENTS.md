# Repository Guidelines

## Project Structure & Module Organization
仓库基于单一 Go 模块 (`go.mod`) 组织，核心入口位于 `cmd/swagger2mcp/main.go`，负责调用 `internal/cli` 中的 Cobra 命令。业务逻辑拆分为 `internal/spec`（规格加载与转换）和 `internal/emitter/{goemitter,npmemitter,pyemitter}`（多语言生成器）。端到端验证存放在 `internal/e2e`，每个子系统的单元测试与源码同目录。根目录包含示例 `swagger.yaml`、自动化脚本 `test_mcp*.sh`、文档（如 `CLAUDE.md`）以及构建产物目录 `bin/` 和临时输出 `tmp/`。运行 `swagger2mcp init` 默认生成的配置样例保存在仓库根部；如需追加研究记录或实验脚本，请新建 `docs/` 或 `tmp/notes` 等隔离目录，避免混入核心源码。

## Build, Test, and Development Commands
常用构建与测试命令集中在 Makefile：`make build` 生成 `bin/swagger2mcp`，`make test` 执行 `go test ./...`，`make e2e` 运行离线端到端场景，`make e2e-online` 在设置 `SWAGGER2MCP_E2E_ONLINE=1` 的情况下验证远程抓取。`make generate-go-sample`、`make generate-npm-sample`、`make generate-python-sample` 依次生成三种语言样例至 `tmp/out-*`。开发调试时可直接使用 Go 工具链，例如 `go run ./cmd/swagger2mcp generate --input swagger.yaml --dry-run` 快速确认输出规划。若需要清理构建与样例输出，执行 `make clean` 或 `make clean-samples`。

## Coding Style & Naming Conventions
遵循 Go 官方风格，提交前执行 `go fmt ./...`，保持制表符缩进与导入排序一致。文件名以领域划分（`loader.go`, `generate.go`），包名短小精炼 (`spec`, `cli`, `emitter`)。导出符号使用驼峰式命名 (`BuildServiceModel`)，内部 helper 以小写开头。模板输出需匹配目标生态习惯：Go 结构为 `cmd/` + `go.mod`，npm 模板包含 `package.json`、`src/index.ts`，Python 模板提供 `pyproject.toml` 与 `src/<package>/__init__.py`。避免引入非 ASCII 字符或混合换行符，提交前可运行 `git diff --check` 捕捉尾随空格。

## Testing Guidelines
测试依赖 Go `testing` 框架；单元测试文件命名为 `*_test.go`，函数以 `Test`/`Benchmark`/`Fuzz` 前缀。对齐现有 table-driven 模式，优先用结构体表格覆盖多规格分支。端到端测试位于 `internal/e2e`; 运行 `go test ./internal/e2e -run E2E -v` 可聚焦生成流程。若改动发射器模板或写入策略，请在 dry-run 断言中验证生成的相对路径，并运行根目录脚本 `./test_mcp.sh` 复查跨语言输出。CI 前至少执行 `make test`，必要时补充本地日志或示例目录帮助审查。

## Commit & Pull Request Guidelines
历史提交保持 72 字符以内的祈使句式，常见前缀有 `feat:`、`fix:` 或中文动词开头，例如 `重构Go和NPM发射器，增强模板系统支持`。建议首行概述核心改动，正文使用空行分隔的项目符号说明细节、关联 Issue 与潜在风险。Pull Request 需包含：变更摘要、背景或需求来源、影响面（涉及的语言/命令）、验证证据（命令输出、dry-run 截图）以及回归风险说明。对生成文件的变动，请附带示例目录结构或简化 diff，帮助审阅者快速复核。

## Security & Configuration Tips
HTTP 抓取默认采用重试与指数退避，避免频繁请求远程服务；在受限环境中可预先下载规格并指向本地路径。加载包含 `file://` 引用的规格需从本地文件启动命令，以便自动允许外部引用。生成器默认避免覆盖现有输出，只有传入 `--force` 时才重写；在 CI 中建议写入 `/tmp/mcp-<lang>` 并于任务结束后清理。若新增外部依赖，请更新 `go.mod` 并说明最小权限需求，同时在 PR 描述中记录额外的环境变量或凭据要求。
