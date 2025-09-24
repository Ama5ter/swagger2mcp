#!/bin/bash

# 简化的 MCP 集成测试脚本 - 直接调用构建产物进行 JSON-RPC 测试

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# 日志函数
log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_mcp() { echo -e "${CYAN}[MCP]${NC} $1"; }
separator() { echo -e "${YELLOW}$(printf '=%.0s' {1..80})${NC}"; }

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"

# 生成测试项目
generate_projects() {
    log_info "生成 MCP 工具项目..."
    
    cd "$PROJECT_ROOT"
    
    # 构建 swagger2mcp 工具
    go build -o bin/swagger2mcp ./cmd/swagger2mcp
    
    # 生成项目
    ./bin/swagger2mcp generate --input swagger.yaml --lang go --out /tmp/mcp-go-test --verbose
    ./bin/swagger2mcp generate --input swagger.yaml --lang npm --out /tmp/mcp-npm-test --verbose
    ./bin/swagger2mcp generate --input swagger.yaml --lang python --out /tmp/mcp-python-test --verbose
    
    log_success "项目生成完成"
}

# 构建项目
build_projects() {
    log_info "构建 MCP 服务器..."
    
    # 构建 Go 版本
    cd /tmp/mcp-go-test
    go mod tidy > /dev/null 2>&1
    go build -o tradedesk-api ./cmd/tradedesk-api
    
    # 构建 NPM 版本
    cd /tmp/mcp-npm-test
    npm install --silent > /dev/null 2>&1
    npm run build > /dev/null 2>&1
    
    # 构建 Python 版本
    cd /tmp/mcp-python-test
    if command -v python3 &> /dev/null; then
        python3 -m pip install -e . > /dev/null 2>&1 || true
        python3 -m pip install -r requirements.txt > /dev/null 2>&1 || true
    else
        log_warning "Python3 未安装，跳过Python版本构建"
    fi
    
    log_success "构建完成"
}

# 测试 MCP 服务器
test_mcp_server() {
    local server_type=$1
    local server_cmd=$2
    local server_dir=$3
    
    log_info "测试 $server_type MCP 服务器..."
    
    cd "$server_dir"
    
    # 创建测试请求文件
    cat > test_requests.json << 'EOT'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"listEndpoints","arguments":{}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"searchEndpoints","arguments":{"keyword":"user"}}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"getEndpointDetails","arguments":{"id":"delete /api/ad-manager/admin/v1/user/:id/"}}}
EOT

    # 运行测试
    log_mcp "启动 $server_type MCP 服务器进行测试..."
    
    local output_file="$(echo ${server_type} | tr '[:upper:]' '[:lower:]')_mcp_output.log"
    
    # 使用 timeout 防止挂起，并通过管道发送请求
    if timeout 15s bash -c "$server_cmd < test_requests.json > $output_file 2>&1"; then
        log_success "$server_type MCP 服务器响应正常"
    else
        log_error "$server_type MCP 服务器测试超时或失败"
        return 1
    fi
    
    # 分析输出结果
    if [ -f "$output_file" ]; then
        local response_count=$(grep -c '"jsonrpc":"2.0"' "$output_file" 2>/dev/null || echo "0")
        local success_count=$(grep -c '"result"' "$output_file" 2>/dev/null || echo "0") 
        local error_count=$(grep -c '"error"' "$output_file" 2>/dev/null || echo "0")
        
        log_mcp "$server_type 测试结果统计:"
        echo "  📊 总响应数: $response_count"
        echo "  ✅ 成功响应: $success_count"
        echo "  ❌ 错误响应: $error_count"
        echo "  📁 详细日志: $server_dir/$output_file"
        
        # 显示部分输出示例
        if [ -s "$output_file" ]; then
            echo
            echo "📝 响应示例 (前800字符):"
            head -c 800 "$output_file"
            echo
            echo "..."
        fi
        
        # 判断是否成功
        if [ "$success_count" -gt 0 ]; then
            log_success "$server_type MCP 服务器集成测试通过"
            return 0
        else
            log_error "$server_type MCP 服务器集成测试失败"
            return 1
        fi
    else
        log_error "$server_type MCP 服务器无输出文件"
        return 1
    fi
}

# 清理函数
cleanup() {
    log_info "清理测试文件..."
    rm -rf /tmp/mcp-go-test /tmp/mcp-npm-test /tmp/mcp-python-test
    log_success "清理完成"
}

# 主函数
main() {
    local go_only=false
    local npm_only=false
    local python_only=false
    local no_cleanup=false
    local cleanup_only=false
    
    # 解析参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                echo "MCP 集成测试脚本"
                echo "用法: $0 [--go-only|--npm-only|--python-only] [--no-cleanup] [-c]"
                echo "选项:"
                echo "  --go-only     只测试 Go 版本"
                echo "  --npm-only    只测试 NPM 版本"
                echo "  --python-only 只测试 Python 版本"
                echo "  --no-cleanup  不清理临时文件"
                echo "  -c            只清理"
                exit 0
                ;;
            --go-only) go_only=true; shift ;;
            --npm-only) npm_only=true; shift ;;
            --python-only) python_only=true; shift ;;
            --no-cleanup) no_cleanup=true; shift ;;
            -c) cleanup_only=true; shift ;;
            *) echo "未知选项: $1"; exit 1 ;;
        esac
    done
    
    if [[ "$cleanup_only" == true ]]; then
        cleanup
        exit 0
    fi
    
    separator
    echo -e "${GREEN}🎯 MCP 集成测试脚本 (简化版)${NC}"
    echo -e "${BLUE}直接调用构建产物进行 JSON-RPC 协议测试${NC}"
    separator
    
    generate_projects
    build_projects
    
    local go_result=0
    local npm_result=0
    local python_result=0
    
    if [[ "$go_only" == true ]]; then
        test_mcp_server "Go" "./tradedesk-api" "/tmp/mcp-go-test" || go_result=$?
    elif [[ "$npm_only" == true ]]; then
        test_mcp_server "NPM" "node dist/index.js" "/tmp/mcp-npm-test" || npm_result=$?
    elif [[ "$python_only" == true ]]; then
        if command -v python3 &> /dev/null; then
            test_mcp_server "Python" "python3 -m tradedesk_api.main" "/tmp/mcp-python-test" || python_result=$?
        else
            log_error "Python3 未安装，无法测试Python版本"
            python_result=1
        fi
    else
        test_mcp_server "Go" "./tradedesk-api" "/tmp/mcp-go-test" || go_result=$?
        test_mcp_server "NPM" "node dist/index.js" "/tmp/mcp-npm-test" || npm_result=$?
        if command -v python3 &> /dev/null; then
            test_mcp_server "Python" "python3 -m tradedesk_api.main" "/tmp/mcp-python-test" || python_result=$?
        else
            log_warning "Python3 未安装，跳过Python版本测试"
        fi
    fi
    
    separator
    log_success "🎉 MCP 集成测试完成！"
    
    if [[ "$go_only" != true && "$npm_only" != true && "$python_only" != true ]]; then
        local total_success=0
        local total_tests=0
        
        [[ $go_result -eq 0 ]] && ((total_success++))
        [[ $npm_result -eq 0 ]] && ((total_success++))
        [[ $python_result -eq 0 ]] && ((total_success++))
        
        if command -v python3 &> /dev/null; then
            total_tests=3
        else
            total_tests=2
        fi
        
        if [[ $total_success -eq $total_tests ]]; then
            echo -e "${GREEN}✅ 所有版本的 MCP 服务器都正常响应 JSON-RPC 请求${NC}"
        else
            echo -e "${RED}❌ 部分测试失败 ($total_success/$total_tests 通过)${NC}"
            [[ $go_result -ne 0 ]] && echo -e "${RED}  - Go 版本测试失败${NC}"
            [[ $npm_result -ne 0 ]] && echo -e "${RED}  - NPM 版本测试失败${NC}"
            [[ $python_result -ne 0 ]] && echo -e "${RED}  - Python 版本测试失败${NC}"
        fi
    fi
    separator
    
    if [[ "$no_cleanup" != true ]]; then
        cleanup
    else
        log_info "临时文件保留在 /tmp/mcp-go-test、/tmp/mcp-npm-test 和 /tmp/mcp-python-test"
    fi
    
    # 返回适当的退出代码
    if [[ "$go_only" == true ]]; then
        exit $go_result
    elif [[ "$npm_only" == true ]]; then
        exit $npm_result
    elif [[ "$python_only" == true ]]; then
        exit $python_result
    else
        if [[ $go_result -eq 0 && $npm_result -eq 0 && $python_result -eq 0 ]]; then
            exit 0
        else
            exit 1
        fi
    fi
}

main "$@"
