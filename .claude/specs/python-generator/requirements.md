# Python3代码生成器需求文档

## 项目介绍

本需求文档定义了在现有swagger2mcp工具中新增Python3代码生成器的功能规范。swagger2mcp工具目前支持Go和Node.js代码生成，本功能将扩展工具以支持Python3 MCP服务器项目的生成。新的Python代码生成器将遵循现有的架构模式，生成符合MCP（Model Context Protocol）规范的Python服务器项目。

## 需求列表

### 需求1：Python代码生成器核心架构

**用户故事：** 作为一个开发者，我希望能够选择python作为生成语言，从而生成基于Python3的MCP服务器项目

#### 验收标准

1. WHEN 用户执行命令 `swagger2mcp generate --lang python` THEN 系统 SHALL 调用Python代码生成器
2. WHEN Python代码生成器被调用 THEN 系统 SHALL 创建一个新的pyemitter包在internal/emitter/pyemitter目录下
3. WHEN pyemitter包被创建 THEN 它 SHALL 包含emitter.go文件，实现Emit函数接口
4. WHEN Emit函数被调用 THEN 它 SHALL 接受ServiceModel和Options参数，返回Result和error
5. WHEN Options结构体被定义 THEN 它 SHALL 包含OutDir、ToolName、PackageName、Force、DryRun、Verbose字段
6. IF generate.go中的runGenerate函数检测到lang为python THEN 它 SHALL 调用pyemitter.Emit函数

### 需求2：Python项目结构生成

**用户故事：** 作为一个开发者，我希望生成的Python项目具有标准的Python项目结构，包含必要的配置文件和目录结构

#### 验收标准

1. WHEN Python项目生成完成 THEN 根目录 SHALL 包含以下文件：setup.py、requirements.txt、README.md、.gitignore、.editorconfig
2. WHEN Python项目生成完成 THEN 项目 SHALL 包含src/工具名/目录作为主包
3. WHEN 主包生成完成 THEN 它 SHALL 包含__init__.py、main.py、server.py文件
4. WHEN 项目结构生成完成 THEN 它 SHALL 包含src/工具名/spec/目录，包含model.py、loader.py、model.json文件
5. WHEN 项目结构生成完成 THEN 它 SHALL 包含src/工具名/mcp/目录，包含__init__.py和methods/子目录
6. WHEN methods目录生成完成 THEN 它 SHALL 包含__init__.py以及各个方法实现文件
7. WHEN 项目结构生成完成 THEN 它 SHALL 包含tests/目录，包含测试文件

### 需求3：Python依赖管理和配置

**用户故事：** 作为一个开发者，我希望生成的Python项目包含正确的依赖管理配置，能够直接安装和运行

#### 验收标准

1. WHEN setup.py文件生成 THEN 它 SHALL 包含项目名称、版本、描述、作者等基本信息
2. WHEN setup.py文件生成 THEN 它 SHALL 在install_requires中声明必要的依赖包
3. WHEN requirements.txt文件生成 THEN 它 SHALL 包含开发和运行所需的所有Python包及版本号
4. WHEN 依赖配置生成 THEN 它 SHALL 包含MCP协议相关的Python库依赖
5. WHEN setup.py生成 THEN 它 SHALL 定义console_scripts入口点，指向main.py的主函数
6. IF 用户指定了PackageName THEN setup.py SHALL 使用指定的包名
7. IF 用户未指定PackageName THEN setup.py SHALL 使用从ServiceModel.Title派生的包名

### 需求4：Python MCP服务器实现

**用户故事：** 作为一个开发者，我希望生成的Python代码能够实现完整的MCP服务器功能，支持标准输入输出通信

#### 验收标准

1. WHEN main.py文件生成 THEN 它 SHALL 实现MCP服务器的标准输入输出协议处理
2. WHEN server.py文件生成 THEN 它 SHALL 实现MCPServer类，处理MCP协议消息
3. WHEN MCP服务器启动 THEN 它 SHALL 支持initialize、ping、tools/list、tools/call等标准MCP方法
4. WHEN 工具调用请求到达 THEN 服务器 SHALL 根据工具名称路由到对应的实现函数
5. WHEN JSON-RPC消息处理 THEN 服务器 SHALL 正确解析请求并返回符合规范的响应
6. WHEN 服务器遇到错误 THEN 它 SHALL 返回正确格式的错误响应
7. WHEN 服务器接收通知消息 THEN 它 SHALL 不返回响应（符合JSON-RPC规范）

### 需求5：Python数据模型和加载器

**用户故事：** 作为一个开发者，我希望生成的Python代码包含完整的数据模型定义和服务模型加载功能

#### 验收标准

1. WHEN model.py文件生成 THEN 它 SHALL 定义ServiceModel、EndpointModel、Schema等数据类
2. WHEN 数据类生成 THEN 它们 SHALL 使用Python的dataclasses或pydantic进行类型注解
3. WHEN loader.py文件生成 THEN 它 SHALL 实现load_service_model函数
4. WHEN load_service_model被调用 THEN 它 SHALL 从嵌入的model.json文件加载数据
5. WHEN model.json文件嵌入 THEN 它 SHALL 包含完整的ServiceModel JSON数据
6. WHEN 数据模型定义 THEN 它们 SHALL 与Go和Node.js版本的数据结构保持一致
7. IF 数据加载失败 THEN loader SHALL 抛出合适的异常

### 需求6：Python MCP方法实现

**用户故事：** 作为一个开发者，我希望生成的Python代码实现所有标准的MCP工具方法，提供API文档查询功能

#### 验收标准

1. WHEN methods/__init__.py生成 THEN 它 SHALL 导出所有方法实现函数
2. WHEN listEndpoints方法实现 THEN 它 SHALL 返回格式化的API概览信息
3. WHEN searchEndpoints方法实现 THEN 它 SHALL 支持关键字、标签、方法、路径模式搜索
4. WHEN getEndpointDetails方法实现 THEN 它 SHALL 支持通过ID或method+path查找端点详情
5. WHEN listSchemas方法实现 THEN 它 SHALL 返回所有Schema的摘要信息
6. WHEN getSchemaDetails方法实现 THEN 它 SHALL 返回指定Schema的详细信息
7. WHEN 方法输出格式化 THEN 所有方法 SHALL 支持中文显示（与现有Go/Node.js版本保持一致）
8. WHEN Schema引用解析 THEN 方法 SHALL 能够解析和显示Schema引用的详细信息

### 需求7：Python项目配置和构建

**用户故事：** 作为一个开发者，我希望生成的Python项目包含完整的开发配置，支持格式化、类型检查和测试

#### 验收标准

1. WHEN pyproject.toml文件生成 THEN 它 SHALL 包含项目构建配置和工具设置
2. WHEN 格式化配置生成 THEN 项目 SHALL 支持black代码格式化工具
3. WHEN 类型检查配置生成 THEN 项目 SHALL 支持mypy静态类型检查
4. WHEN 测试配置生成 THEN 项目 SHALL 支持pytest测试框架
5. WHEN Makefile生成 THEN 它 SHALL 包含install、test、format、lint、build等常用目标
6. WHEN .gitignore生成 THEN 它 SHALL 包含Python项目的标准忽略规则
7. WHEN 开发环境配置完成 THEN 开发者 SHALL 能够通过make命令执行各种开发任务

### 需求8：Python测试文件生成

**用户故事：** 作为一个开发者，我希望生成的Python项目包含基本的测试用例，验证MCP方法的正确性

#### 验收标准

1. WHEN tests/test_mcp_methods.py生成 THEN 它 SHALL 包含所有MCP方法的基本测试
2. WHEN 测试运行 THEN test_list_and_search函数 SHALL 验证端点列表和搜索功能
3. WHEN 测试运行 THEN test_schema_details函数 SHALL 验证Schema查询功能
4. WHEN 测试运行 THEN test_endpoint_details函数 SHALL 验证端点详情查询功能
5. WHEN 测试执行 THEN 所有测试 SHALL 使用加载的ServiceModel进行验证
6. WHEN 测试框架配置 THEN 项目 SHALL 支持通过pytest运行测试
7. IF ServiceModel为空或无效 THEN 测试 SHALL 提供合适的错误处理

### 需求9：Python CLI工具集成

**用户故事：** 作为一个开发者，我希望能够在swagger2mcp的命令行界面中无缝使用python选项

#### 验收标准

1. WHEN generate.go的Lang验证逻辑更新 THEN 它 SHALL 接受"python"作为有效选项
2. WHEN 帮助文档更新 THEN --lang参数说明 SHALL 包含"python"选项
3. WHEN runGenerate函数执行 THEN 它 SHALL 为lang="python"添加相应的case分支
4. WHEN Python生成器调用 THEN 它 SHALL 传递正确的Options参数
5. WHEN 错误处理实现 THEN Python生成器错误 SHALL 被正确包装和显示
6. WHEN dry-run模式执行 THEN Python生成器 SHALL 正确显示计划生成的文件列表
7. IF 生成成功 THEN 用户 SHALL 看到与Go/Node.js版本一致的成功消息格式

### 需求10：Python代码质量和兼容性

**用户故事：** 作为一个开发者，我希望生成的Python代码具有高质量，兼容Python 3.8+版本

#### 验收标准

1. WHEN Python代码生成 THEN 所有代码 SHALL 兼容Python 3.8及以上版本
2. WHEN 类型注解使用 THEN 代码 SHALL 使用标准的typing模块类型
3. WHEN 代码风格实现 THEN 生成的代码 SHALL 符合PEP 8规范
4. WHEN 文档字符串生成 THEN 函数和类 SHALL 包含适当的docstring
5. WHEN 异常处理实现 THEN 代码 SHALL 使用适当的异常类型和处理
6. WHEN 日志记录实现 THEN 代码 SHALL 使用Python标准logging模块
7. WHEN 代码生成完成 THEN 项目结构 SHALL 遵循Python包的标准组织方式
8. WHEN 依赖管理 THEN 生成的项目 SHALL 避免使用过于新颖或不稳定的依赖包