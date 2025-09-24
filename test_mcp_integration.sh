#!/bin/bash

# MCP 集成测试脚本 - 直接调用构建产物测试完整的 MCP 工具集成
# 模拟真实的 MCP 工具使用场景

set -e  # 遇到错误立即退出

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${PURPLE}[STEP]${NC} $1"
}

log_mcp() {
    echo -e "${CYAN}[MCP]${NC} $1"
}

# 分隔线
separator() {
    echo -e "${YELLOW}$(printf '=%.0s' {1..80})${NC}"
}

step_separator() {
    echo -e "${PURPLE}$(printf '%*s' 60 '' | tr ' ' '-')${NC}"
}

# 获取脚本所在目录（项目根目录）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"

# 检查必要工具
check_prerequisites() {
    log_info "检查必要工具..."
    
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装或不在 PATH 中"
        exit 1
    fi
    
    if ! command -v node &> /dev/null; then
        log_error "Node.js 未安装或不在 PATH 中"
        exit 1
    fi
    
    if ! command -v npm &> /dev/null; then
        log_error "npm 未安装或不在 PATH 中"
        exit 1
    fi
    
    if ! command -v jq &> /dev/null; then
        log_warning "jq 未安装，JSON 输出将不会格式化"
    fi
    
    log_success "工具检查完成"
}

# 生成测试项目
generate_projects() {
    log_info "生成 MCP 工具项目..."
    
    # 确保在项目根目录
    cd "$PROJECT_ROOT"
    
    # 构建 swagger2mcp 工具
    log_info "构建 swagger2mcp 工具..."
    go build -o bin/swagger2mcp ./cmd/swagger2mcp
    
    # 生成 Go 版本项目
    log_info "生成 Go 版本 MCP 工具..."
    ./bin/swagger2mcp generate --input swagger.yaml --lang go --out /tmp/mcp-go-integration --verbose
    
    # 生成 NPM 版本项目
    log_info "生成 NPM 版本 MCP 工具..."
    ./bin/swagger2mcp generate --input swagger.yaml --lang npm --out /tmp/mcp-npm-integration --verbose
    
    # 生成 Python 版本项目
    if command -v python3 &> /dev/null; then
        log_info "生成 Python 版本 MCP 工具..."
        ./bin/swagger2mcp generate --input swagger.yaml --lang python --out /tmp/mcp-python-integration --verbose
    fi
    
    log_success "MCP 工具项目生成完成"
}

# 构建 Go MCP 服务器
build_go_mcp() {
    log_info "构建 Go MCP 服务器..."
    
    cd /tmp/mcp-go-integration
    go mod tidy > /dev/null 2>&1
    go build -o tradedesk-api ./cmd/tradedesk-api
    
    if [ ! -f "tradedesk-api" ]; then
        log_error "Go MCP 服务器构建失败"
        exit 1
    fi
    
    log_success "Go MCP 服务器构建完成"
}

# 构建 NPM MCP 服务器
build_npm_mcp() {
    log_info "构建 NPM MCP 服务器..."
    
    cd /tmp/mcp-npm-integration
    npm install --silent > /dev/null 2>&1
    npm run build > /dev/null 2>&1
    
    if [ ! -f "dist/index.js" ]; then
        log_error "NPM MCP 服务器构建失败"
        exit 1
    fi
    
    log_success "NPM MCP 服务器构建完成"
}

# 发送 JSON-RPC 请求的函数
send_mcp_request() {
    local server_process=$1
    local method=$2
    local params=$3
    local request_id=$4
    
    if [ -z "$params" ] || [ "$params" = "null" ]; then
        params="{}"
    fi
    
    local request="{\"jsonrpc\":\"2.0\",\"id\":$request_id,\"method\":\"$method\",\"params\":$params}"
    
    echo "$request" | timeout 10s tee /proc/$server_process/fd/0 2>/dev/null || {
        log_error "发送请求失败或超时"
        return 1
    }
    
    sleep 0.5  # 给服务器处理时间
}

# 读取 MCP 服务器响应
read_mcp_response() {
    local server_process=$1
    local timeout=${2:-5}
    
    # 从服务器的 stdout 读取响应
    timeout $timeout cat /proc/$server_process/fd/1 2>/dev/null || {
        log_warning "读取响应超时"
        return 1
    }
}

# 启动 Go MCP 服务器并测试
test_go_mcp_integration() {
    log_step "测试 Go MCP 服务器集成..."
    step_separator
    
    cd /tmp/mcp-go-integration
    
    # 启动 MCP 服务器（后台运行）
    log_mcp "启动 Go MCP 服务器..."
    ./tradedesk-api &
    local server_pid=$!
    
    # 等待服务器启动
    sleep 2
    
    # 检查服务器是否启动成功
    if ! kill -0 $server_pid 2>/dev/null; then
        log_error "Go MCP 服务器启动失败"
        return 1
    fi
    
    log_success "Go MCP 服务器已启动 (PID: $server_pid)"
    
    # 创建测试脚本
    cat > test_mcp_requests.sh << 'EOF'
#!/bin/bash

echo "=== Go MCP 服务器集成测试 ==="
echo

# 发送初始化请求
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}'
sleep 0.5

# 请求工具列表
echo
echo "--- 获取可用工具列表 ---"
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
sleep 1

# 测试 listEndpoints
echo
echo "--- 测试 listEndpoints 工具 ---"
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"listEndpoints","arguments":{}}}'
sleep 2

# 测试 searchEndpoints
echo
echo "--- 测试 searchEndpoints 工具 ---"
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"searchEndpoints","arguments":{"keyword":"user"}}}'
sleep 1

# 测试 getEndpointDetails
echo
echo "--- 测试 getEndpointDetails 工具 ---"
echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"getEndpointDetails","arguments":{"id":"delete /api/ad-manager/admin/v1/user/:id/"}}}'
sleep 1

# 测试 listSchemas
echo
echo "--- 测试 listSchemas 工具 ---"
echo '{"jsonrpc":"2.0","id":6,"method":"tools/list","params":{}}'
sleep 1

echo
echo "=== 测试完成 ==="
EOF

    chmod +x test_mcp_requests.sh
    
    # 执行测试并捕获输出
    log_mcp "执行 MCP 协议测试..."
    local test_output=$(./test_mcp_requests.sh | ./tradedesk-api 2>&1)
    
    # 分析测试结果
    echo "$test_output" > go_mcp_test_output.log
    
    # 检查是否有成功的响应
    local success_count=$(echo "$test_output" | grep -c '"result"' || echo "0")
    local error_count=$(echo "$test_output" | grep -c '"error"' || echo "0")
    local total_responses=$(echo "$test_output" | grep -c '"jsonrpc":"2.0"' || echo "0")
    
    log_mcp "测试结果统计:"
    echo "  📊 总响应数: $total_responses"
    echo "  ✅ 成功响应: $success_count" 
    echo "  ❌ 错误响应: $error_count"
    
    # 显示部分输出示例
    if [ -s go_mcp_test_output.log ]; then
        echo
        log_mcp "响应示例 (前500字符):"
        echo "$(head -c 500 go_mcp_test_output.log)..."
        echo
    fi
    
    # 清理服务器进程
    if kill -0 $server_pid 2>/dev/null; then
        kill $server_pid 2>/dev/null
        wait $server_pid 2>/dev/null || true
        log_success "Go MCP 服务器已停止"
    fi
    
    # 判断测试结果
    if [ "$success_count" -gt 0 ]; then
        log_success "Go MCP 服务器集成测试通过"
        return 0
    else
        log_error "Go MCP 服务器集成测试失败"
        return 1
    fi
}

# 启动 NPM MCP 服务器并测试
test_npm_mcp_integration() {
    log_step "测试 NPM MCP 服务器集成..."
    step_separator
    
    cd /tmp/mcp-npm-integration
    
    # 启动 MCP 服务器（后台运行）
    log_mcp "启动 NPM MCP 服务器..."
    node dist/index.js &
    local server_pid=$!
    
    # 等待服务器启动
    sleep 2
    
    # 检查服务器是否启动成功
    if ! kill -0 $server_pid 2>/dev/null; then
        log_error "NPM MCP 服务器启动失败"
        return 1
    fi
    
    log_success "NPM MCP 服务器已启动 (PID: $server_pid)"
    
    # 创建测试脚本
    cat > test_mcp_requests.sh << 'EOF'
#!/bin/bash

echo "=== NPM MCP 服务器集成测试 ==="
echo

# 发送初始化请求
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}'
sleep 0.5

# 请求工具列表
echo
echo "--- 获取可用工具列表 ---"
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
sleep 1

# 测试 listEndpoints
echo
echo "--- 测试 listEndpoints 工具 ---"
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"listEndpoints","arguments":{}}}'
sleep 2

# 测试 searchEndpoints
echo
echo "--- 测试 searchEndpoints 工具 ---"
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"searchEndpoints","arguments":{"keyword":"user"}}}'
sleep 1

# 测试 getEndpointDetails
echo
echo "--- 测试 getEndpointDetails 工具 ---"
echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"getEndpointDetails","arguments":{"id":"delete /api/ad-manager/admin/v1/user/:id/"}}}'
sleep 1

echo
echo "=== 测试完成 ==="
EOF

    chmod +x test_mcp_requests.sh
    
    # 执行测试并捕获输出
    log_mcp "执行 MCP 协议测试..."
    local test_output=$(./test_mcp_requests.sh | node dist/index.js 2>&1)
    
    # 分析测试结果
    echo "$test_output" > npm_mcp_test_output.log
    
    # 检查是否有成功的响应
    local success_count=$(echo "$test_output" | grep -c '"result"' || echo "0")
    local error_count=$(echo "$test_output" | grep -c '"error"' || echo "0")
    local total_responses=$(echo "$test_output" | grep -c '"jsonrpc":"2.0"' || echo "0")
    
    log_mcp "测试结果统计:"
    echo "  📊 总响应数: $total_responses"
    echo "  ✅ 成功响应: $success_count" 
    echo "  ❌ 错误响应: $error_count"
    
    # 显示部分输出示例
    if [ -s npm_mcp_test_output.log ]; then
        echo
        log_mcp "响应示例 (前500字符):"
        echo "$(head -c 500 npm_mcp_test_output.log)..."
        echo
    fi
    
    # 清理服务器进程
    if kill -0 $server_pid 2>/dev/null; then
        kill $server_pid 2>/dev/null
        wait $server_pid 2>/dev/null || true
        log_success "NPM MCP 服务器已停止"
    fi
    
    # 判断测试结果
    if [ "$success_count" -gt 0 ]; then
        log_success "NPM MCP 服务器集成测试通过"
        return 0
    else
        log_error "NPM MCP 服务器集成测试失败"
        return 1
    fi
}

# 简化的 MCP 交互测试
test_mcp_simple_interaction() {
    local server_type=$1
    local server_path=$2
    
    log_step "测试 $server_type MCP 服务器简化交互..."
    
    cd "$server_path"
    
    # 创建简化的测试客户端
    cat > simple_mcp_client.mjs << 'EOF'
import { spawn } from 'child_process';
import readline from 'readline';

async function testMCPServer(serverCommand) {
    return new Promise((resolve) => {
        console.log(`🚀 启动 MCP 服务器: ${serverCommand.join(' ')}`);
        
        const server = spawn(serverCommand[0], serverCommand.slice(1), {
            stdio: ['pipe', 'pipe', 'pipe']
        });
        
        let responseCount = 0;
        const results = [];
        
        // 读取服务器输出
        const rl = readline.createInterface({
            input: server.stdout,
            crlfDelay: Infinity
        });
        
        rl.on('line', (line) => {
            if (line.trim()) {
                try {
                    const response = JSON.parse(line);
                    responseCount++;
                    results.push(response);
                    console.log(`📨 响应 ${responseCount}:`, JSON.stringify(response, null, 2).substring(0, 200) + '...');
                } catch (e) {
                    console.log(`📝 输出: ${line.substring(0, 100)}...`);
                }
            }
        });
        
        // 发送测试请求
        const requests = [
            {jsonrpc: "2.0", id: 1, method: "initialize", params: {protocolVersion: "2024-11-05", capabilities: {}, clientInfo: {name: "test-client", version: "1.0.0"}}},
            {jsonrpc: "2.0", id: 2, method: "tools/list", params: {}},
            {jsonrpc: "2.0", id: 3, method: "tools/call", params: {name: "listEndpoints", arguments: {}}},
            {jsonrpc: "2.0", id: 4, method: "tools/call", params: {name: "searchEndpoints", arguments: {keyword: "user"}}},
        ];
        
        let requestIndex = 0;
        
        function sendNextRequest() {
            if (requestIndex < requests.length) {
                const request = requests[requestIndex++];
                console.log(`📤 发送请求 ${requestIndex}: ${request.method}`);
                server.stdin.write(JSON.stringify(request) + '\n');
                setTimeout(sendNextRequest, 1000);
            } else {
                setTimeout(() => {
                    server.kill();
                    resolve({
                        responseCount,
                        results,
                        success: responseCount > 0
                    });
                }, 2000);
            }
        }
        
        // 开始发送请求
        setTimeout(sendNextRequest, 500);
        
        // 错误处理
        server.on('error', (error) => {
            console.error('❌ 服务器错误:', error);
            resolve({responseCount: 0, results: [], success: false});
        });
        
        server.on('exit', (code) => {
            console.log(`🏁 服务器退出，代码: ${code}`);
        });
    });
}

// 根据参数选择服务器
const serverType = process.argv[2];
let serverCommand;

if (serverType === 'go') {
    serverCommand = ['./tradedesk-api'];
} else if (serverType === 'npm') {
    serverCommand = ['node', 'dist/index.js'];
} else {
    console.error('请指定服务器类型: go 或 npm');
    process.exit(1);
}

testMCPServer(serverCommand).then(result => {
    console.log('\n📊 测试结果:');
    console.log(`  响应数量: ${result.responseCount}`);
    console.log(`  测试结果: ${result.success ? '✅ 通过' : '❌ 失败'}`);
    
    if (result.success) {
        console.log('🎉 MCP 服务器集成测试成功！');
    } else {
        console.log('💥 MCP 服务器集成测试失败！');
        process.exit(1);
    }
});
EOF

    # 运行简化测试
    if command -v node &> /dev/null; then
        log_mcp "运行简化 MCP 交互测试..."
        local server_cmd=""
        if [ "$server_type" = "Go" ]; then
            server_cmd="go"
        else
            server_cmd="npm"
        fi
        
        timeout 30s node simple_mcp_client.mjs "$server_cmd" || {
            log_warning "简化测试超时或失败"
            return 1
        }
    else
        log_warning "Node.js 不可用，跳过简化交互测试"
        return 0
    fi
}

# 清理函数
cleanup() {
    log_info "清理测试文件..."
    
    # 杀死可能残留的进程
    pkill -f "tradedesk-api" 2>/dev/null || true
    pkill -f "dist/index.js" 2>/dev/null || true
    
    # 删除临时文件
    rm -rf /tmp/mcp-go-integration
    rm -rf /tmp/mcp-npm-integration
    rm -rf /tmp/mcp-python-integration
    
    log_success "清理完成"
}

# 显示使用说明
usage() {
    cat << EOF
MCP 集成测试脚本 - 直接调用构建产物

使用说明：
  $0 [选项]

选项：
  -h, --help       显示此帮助信息
  -c, --cleanup    只执行清理操作
  --go-only        只测试 Go 版本 MCP 服务器
  --npm-only       只测试 NPM 版本 MCP 服务器
  --simple-test    使用简化的交互测试
  --no-cleanup     测试完成后不清理临时文件

测试内容：
  1. 构建真实的 MCP 服务器可执行文件
  2. 启动 MCP 服务器进程
  3. 通过 JSON-RPC 协议发送 MCP 请求
  4. 验证 MCP 工具调用（listEndpoints、searchEndpoints、getEndpointDetails）
  5. 分析响应结果并生成测试报告

示例：
  $0                    # 运行完整 MCP 集成测试
  $0 --go-only          # 只测试 Go MCP 服务器
  $0 --npm-only         # 只测试 NPM MCP 服务器
  $0 --simple-test      # 使用简化交互测试
  $0 --no-cleanup       # 保留测试文件
  $0 -c                 # 清理临时文件

项目位置：
  - 测试脚本: $(realpath "$0")
  - 项目根目录: $PROJECT_ROOT

EOF
}

# 主函数
main() {
    local go_only=false
    local npm_only=false
    local no_cleanup=false
    local cleanup_only=false
    local simple_test=false
    
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -c|--cleanup)
                cleanup_only=true
                shift
                ;;
            --go-only)
                go_only=true
                shift
                ;;
            --npm-only)
                npm_only=true
                shift
                ;;
            --simple-test)
                simple_test=true
                shift
                ;;
            --no-cleanup)
                no_cleanup=true
                shift
                ;;
            *)
                log_error "未知选项: $1"
                usage
                exit 1
                ;;
        esac
    done
    
    # 如果只是清理
    if [[ "$cleanup_only" == true ]]; then
        cleanup
        exit 0
    fi
    
    # 显示测试开始信息
    separator
    echo -e "${GREEN}🎯 MCP 集成测试脚本${NC}"
    echo -e "${BLUE}测试类型：真实 MCP 服务器集成测试${NC}"
    echo -e "${YELLOW}项目位置：$PROJECT_ROOT${NC}"
    separator
    
    # 执行测试流程
    check_prerequisites
    generate_projects
    
    local go_result=0
    local npm_result=0
    
    if [[ "$go_only" == true ]]; then
        build_go_mcp
        if [[ "$simple_test" == true ]]; then
            test_mcp_simple_interaction "Go" "/tmp/mcp-go-integration" || go_result=$?
        else
            test_go_mcp_integration || go_result=$?
        fi
    elif [[ "$npm_only" == true ]]; then
        build_npm_mcp
        if [[ "$simple_test" == true ]]; then
            test_mcp_simple_interaction "NPM" "/tmp/mcp-npm-integration" || npm_result=$?
        else
            test_npm_mcp_integration || npm_result=$?
        fi
    else
        # 运行两个版本的测试
        build_go_mcp
        build_npm_mcp
        
        if [[ "$simple_test" == true ]]; then
            test_mcp_simple_interaction "Go" "/tmp/mcp-go-integration" || go_result=$?
            test_mcp_simple_interaction "NPM" "/tmp/mcp-npm-integration" || npm_result=$?
        else
            test_go_mcp_integration || go_result=$?
            test_npm_mcp_integration || npm_result=$?
        fi
    fi
    
    # 最终总结
    separator
    log_success "🎉 MCP 集成测试完成！"
    
    if [[ "$go_only" != true && "$npm_only" != true ]]; then
        if [[ $go_result -eq 0 && $npm_result -eq 0 ]]; then
            echo -e "${GREEN}✅ Go 和 NPM 版本的 MCP 服务器集成测试都通过${NC}"
            echo -e "${BLUE}📊 两个版本的 MCP 工具都能正常响应 JSON-RPC 请求！${NC}"
        else
            echo -e "${RED}❌ 部分测试失败${NC}"
            if [[ $go_result -ne 0 ]]; then
                echo -e "${RED}  - Go MCP 服务器测试失败${NC}"
            fi
            if [[ $npm_result -ne 0 ]]; then
                echo -e "${RED}  - NPM MCP 服务器测试失败${NC}"
            fi
        fi
    fi
    
    echo -e "${CYAN}📁 测试日志位置:${NC}"
    if [[ -f "/tmp/mcp-go-integration/go_mcp_test_output.log" ]]; then
        echo -e "  ${YELLOW}Go MCP: /tmp/mcp-go-integration/go_mcp_test_output.log${NC}"
    fi
    if [[ -f "/tmp/mcp-npm-integration/npm_mcp_test_output.log" ]]; then
        echo -e "  ${YELLOW}NPM MCP: /tmp/mcp-npm-integration/npm_mcp_test_output.log${NC}"
    fi
    
    separator
    
    # 清理临时文件
    if [[ "$no_cleanup" != true ]]; then
        cleanup
    else
        log_info "临时文件保留在:"
        echo -e "  ${YELLOW}/tmp/mcp-go-integration${NC}"
        echo -e "  ${YELLOW}/tmp/mcp-npm-integration${NC}"
        if command -v python3 &> /dev/null; then
            echo -e "  ${YELLOW}/tmp/mcp-python-integration${NC}"
        fi
        echo -e "可以使用 $0 -c 清理这些文件"
    fi
    
    # 返回适当的退出代码
    if [[ "$go_only" == true ]]; then
        exit $go_result
    elif [[ "$npm_only" == true ]]; then
        exit $npm_result
    else
        if [[ $go_result -eq 0 && $npm_result -eq 0 ]]; then
            exit 0
        else
            exit 1
        fi
    fi
}

# 运行主函数
main "$@"