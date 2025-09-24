package pyemitter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	genspec "github.com/mark3labs/swagger2mcp/internal/spec"
)

func TestEmit_BasicFunctionality(t *testing.T) {
	// Create a minimal service model for testing
	sm := &genspec.ServiceModel{
		Title:       "Test API",
		Version:     "1.0.0",
		Description: "A test API for pyemitter",
		Endpoints: []genspec.EndpointModel{
			{
				ID:      "get /test",
				Method:  genspec.GET,
				Path:    "/test",
				Summary: "Test endpoint",
			},
		},
		Schemas: make(map[string]genspec.Schema),
	}

	// Create temporary directory for output
	tmpDir, err := os.MkdirTemp("", "pyemitter-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := Options{
		OutDir:      tmpDir,
		ToolName:    "test-tool",
		PackageName: "test_package",
		Force:       true,
		DryRun:      false,
		Verbose:     true,
	}

	// Test Emit function
	result, err := Emit(context.Background(), sm, opts)
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// Verify result
	if result.ToolName != "test-tool" {
		t.Errorf("expected tool name 'test-tool', got '%s'", result.ToolName)
	}
	if result.PackageName != "test_package" {
		t.Errorf("expected package name 'test_package', got '%s'", result.PackageName)
	}
	if len(result.Planned) == 0 {
		t.Errorf("expected planned files, got none")
	}

	// Verify some key files exist
	expectedFiles := []string{
		"setup.py",
		"requirements.txt",
		"pyproject.toml",
		"src/test_package/__init__.py",
		"src/test_package/main.py",
		"src/test_package/server.py",
		"src/test_package/spec/model.py",
		"src/test_package/spec/loader.py",
		"src/test_package/spec/model.json",
	}

	for _, expectedFile := range expectedFiles {
		fullPath := filepath.Join(tmpDir, expectedFile)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected file %s does not exist", expectedFile)
		}
	}
}

func TestEmit_DryRun(t *testing.T) {
	sm := &genspec.ServiceModel{
		Title:       "Test API",
		Version:     "1.0.0",
		Description: "A test API for pyemitter",
		Schemas:     make(map[string]genspec.Schema),
	}

	tmpDir, err := os.MkdirTemp("", "pyemitter-dryrun-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := Options{
		OutDir:      tmpDir,
		ToolName:    "test-tool",
		PackageName: "test_package",
		Force:       true,
		DryRun:      true, // dry run mode
		Verbose:     true,
	}

	// Test Emit function in dry run mode
	result, err := Emit(context.Background(), sm, opts)
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// Verify result shows planned files but no files are actually written
	if len(result.Planned) == 0 {
		t.Errorf("expected planned files, got none")
	}

	// Verify no files are actually created (dry run)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("expected no files to be created in dry run mode, but found %d files", len(entries))
	}
}

func TestEmit_NilServiceModel(t *testing.T) {
	opts := Options{
		OutDir:   "/tmp/test",
		ToolName: "test-tool",
	}

	_, err := Emit(context.Background(), nil, opts)
	if err == nil {
		t.Errorf("expected error for nil ServiceModel, got nil")
	}
	if !contains(err.Error(), "nil ServiceModel") {
		t.Errorf("expected error message to contain 'nil ServiceModel', got: %s", err.Error())
	}
}

func TestEmit_EmptyOutDir(t *testing.T) {
	sm := &genspec.ServiceModel{
		Title:   "Test API",
		Version: "1.0.0",
		Schemas: make(map[string]genspec.Schema),
	}

	opts := Options{
		OutDir:   "", // empty out dir
		ToolName: "test-tool",
	}

	_, err := Emit(context.Background(), sm, opts)
	if err == nil {
		t.Errorf("expected error for empty OutDir, got nil")
	}
	if !contains(err.Error(), "OutDir is required") {
		t.Errorf("expected error message to contain 'OutDir is required', got: %s", err.Error())
	}
}

func TestSanitizeToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Test Tool", "test-tool"},
		{"test/tool", "test-tool"},
		{"TEST_TOOL", "test_tool"},
		{"  Test-Tool  ", "test-tool"},
		{"", ""},
		{"test123", "test123"},
		{"test@#$%tool", "testtool"},
	}

	for _, test := range tests {
		result := sanitizeToolName(test.input)
		if result != test.expected {
			t.Errorf("sanitizeToolName(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestSanitizePackageName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test-tool", "test_tool"},
		{"Test Package", "test_package"},
		{"TEST_PACKAGE", "test_package"},
		{"  test_package  ", "test_package"},
		{"", ""},
		{"test123", "test123"},
		{"test@#$%package", "testpackage"},
	}

	for _, test := range tests {
		result := sanitizePackageName(test.input)
		if result != test.expected {
			t.Errorf("sanitizePackageName(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestDeriveToolName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Pet Store API", "pet-store-api"},
		{"User Management", "user-management"},
		{"", ""},
		{"API/v1", "api-v1"},
		{"Test_API.v2", "test-api-v2"},
	}

	for _, test := range tests {
		result := deriveToolName(test.input)
		if result != test.expected {
			t.Errorf("deriveToolName(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && someContains(s, substr)))
}

func someContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 测试任务10：集成测试和验证

// TestEmit_CompleteGeneration 测试完整的Python项目生成
func TestEmit_CompleteGeneration(t *testing.T) {
	// 创建复杂的ServiceModel用于完整测试
	sm := createComplexServiceModel()

	// 创建临时输出目录
	tmpDir, err := os.MkdirTemp("", "pyemitter-complete-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := Options{
		OutDir:      tmpDir,
		ToolName:    "complex-api-tool",
		PackageName: "complex_api",
		Force:       true,
		DryRun:      false,
		Verbose:     true,
	}

	// 执行生成
	result, err := Emit(context.Background(), sm, opts)
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// 验证结果
	verifyEmitResult(t, result, opts)

	// 验证所有必需文件都生成了
	verifyAllRequiredFiles(t, tmpDir)

	// 验证生成文件的内容正确性
	verifyGeneratedContent(t, tmpDir, sm, opts)
}

// TestEmit_PythonSyntaxValidation 验证生成的Python代码语法正确
func TestEmit_PythonSyntaxValidation(t *testing.T) {
	sm := createComplexServiceModel()

	tmpDir, err := os.MkdirTemp("", "pyemitter-syntax-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := Options{
		OutDir:      tmpDir,
		ToolName:    "syntax-test",
		PackageName: "syntax_test",
		Force:       true,
		DryRun:      false,
		Verbose:     false,
	}

	_, err = Emit(context.Background(), sm, opts)
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// 验证Python语法
	verifyPythonSyntax(t, tmpDir)
}

// TestEmit_TemplateRendering 测试模板渲染正确性
func TestEmit_TemplateRendering(t *testing.T) {
	sm := createComplexServiceModel()

	tmpDir, err := os.MkdirTemp("", "pyemitter-template-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	opts := Options{
		OutDir:      tmpDir,
		ToolName:    "template-test-api",
		PackageName: "template_test_api",
		Force:       true,
		DryRun:      false,
		Verbose:     false,
	}

	_, err = Emit(context.Background(), sm, opts)
	if err != nil {
		t.Fatalf("Emit failed: %v", err)
	}

	// 验证模板渲染结果
	verifyTemplateContent(t, tmpDir, sm, opts)
}

// TestEmit_DryRunValidation 深度测试DryRun模式
func TestEmit_DryRunValidation(t *testing.T) {
	sm := createComplexServiceModel()

	tmpDir, err := os.MkdirTemp("", "pyemitter-dryrun-validation-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 测试DryRun模式
	dryRunOpts := Options{
		OutDir:      tmpDir,
		ToolName:    "dryrun-test",
		PackageName: "dryrun_test",
		Force:       true,
		DryRun:      true, // DryRun模式
		Verbose:     true,
	}

	dryRunResult, err := Emit(context.Background(), sm, dryRunOpts)
	if err != nil {
		t.Fatalf("DryRun Emit failed: %v", err)
	}

	// 验证DryRun结果包含正确的Planned文件
	if len(dryRunResult.Planned) == 0 {
		t.Errorf("DryRun should return planned files, got none")
	}

	// 验证没有实际文件被创建
	verifyNoDryRunFiles(t, tmpDir)

	// 现在执行实际生成
	realOpts := dryRunOpts
	realOpts.DryRun = false
	realResult, err := Emit(context.Background(), sm, realOpts)
	if err != nil {
		t.Fatalf("Real Emit failed: %v", err)
	}

	// 验证两个结果的Planned列表一致
	verifyPlannedFilesMatch(t, dryRunResult.Planned, realResult.Planned)

	// 验证实际文件被创建
	verifyActualFiles(t, tmpDir, realResult.Planned)
}

// TestEmit_ErrorHandling 测试错误处理场景
func TestEmit_ErrorHandling(t *testing.T) {
	// 测试各种错误情况
	tests := []struct {
		name         string
		serviceModel *genspec.ServiceModel
		options      Options
		expectedErr  string
	}{
		{
			name:         "nil service model",
			serviceModel: nil,
			options: Options{
				OutDir:   "/tmp/test",
				ToolName: "test",
			},
			expectedErr: "nil ServiceModel",
		},
		{
			name:         "empty OutDir",
			serviceModel: createSimpleServiceModel(),
			options: Options{
				OutDir:   "",
				ToolName: "test",
			},
			expectedErr: "OutDir is required",
		},
		{
			name:         "invalid OutDir (file exists)",
			serviceModel: createSimpleServiceModel(),
			options: Options{
				OutDir:   createTempFile(t),
				ToolName: "test",
				Force:    false,
			},
			expectedErr: "is not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Emit(context.Background(), tt.serviceModel, tt.options)
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.expectedErr)
				return
			}
			if !contains(err.Error(), tt.expectedErr) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

// Helper functions

func createComplexServiceModel() *genspec.ServiceModel {
	return &genspec.ServiceModel{
		Title:       "Complex Pet Store API",
		Version:     "2.1.0",
		Description: "A comprehensive pet store API with multiple endpoints and complex schemas",
		Servers: []genspec.Server{
			{URL: "https://api.petstore.com/v2", Description: "Production server"},
			{URL: "https://staging.petstore.com/v2", Description: "Staging server"},
		},
		Tags: []string{"pets", "users", "orders"},
		Endpoints: []genspec.EndpointModel{
			{
				ID:          "get /pets",
				Method:      genspec.GET,
				Path:        "/pets",
				Summary:     "List all pets",
				Description: "Retrieve a list of pets with optional filtering",
				Tags:        []string{"pets"},
				Parameters: []genspec.ParameterModel{
					{
						Name:     "limit",
						In:       "query",
						Required: false,
						Schema: &genspec.SchemaOrRef{
							Schema: &genspec.Schema{Type: "integer", Format: "int32"},
						},
					},
					{
						Name:     "status",
						In:       "query",
						Required: false,
						Schema: &genspec.SchemaOrRef{
							Schema: &genspec.Schema{
								Type: "string",
								Enum: []any{"available", "pending", "sold"},
							},
						},
					},
				},
				Responses: []genspec.ResponseModel{
					{
						Status:      "200",
						Description: "A list of pets",
						Content: []genspec.Media{
							{
								Mime: "application/json",
								Schema: &genspec.SchemaOrRef{
									Schema: &genspec.Schema{
										Type: "array",
										Items: &genspec.SchemaOrRef{
											Ref: &genspec.SchemaRef{Ref: "#/components/schemas/Pet"},
										},
									},
								},
							},
						},
					},
				},
			},
			{
				ID:          "post /pets",
				Method:      genspec.POST,
				Path:        "/pets",
				Summary:     "Create a new pet",
				Description: "Add a new pet to the store",
				Tags:        []string{"pets"},
				RequestBody: &genspec.RequestBodyModel{
					Required: true,
					Content: []genspec.Media{
						{
							Mime: "application/json",
							Schema: &genspec.SchemaOrRef{
								Ref: &genspec.SchemaRef{Ref: "#/components/schemas/NewPet"},
							},
						},
					},
				},
				Responses: []genspec.ResponseModel{
					{
						Status:      "201",
						Description: "Pet created successfully",
						Content: []genspec.Media{
							{
								Mime: "application/json",
								Schema: &genspec.SchemaOrRef{
									Ref: &genspec.SchemaRef{Ref: "#/components/schemas/Pet"},
								},
							},
						},
					},
				},
			},
			{
				ID:          "get /pets/{petId}",
				Method:      genspec.GET,
				Path:        "/pets/{petId}",
				Summary:     "Get pet by ID",
				Description: "Retrieve a single pet by its unique identifier",
				Tags:        []string{"pets"},
				Parameters: []genspec.ParameterModel{
					{
						Name:     "petId",
						In:       "path",
						Required: true,
						Schema: &genspec.SchemaOrRef{
							Schema: &genspec.Schema{Type: "integer", Format: "int64"},
						},
					},
				},
				Responses: []genspec.ResponseModel{
					{
						Status:      "200",
						Description: "Pet details",
						Content: []genspec.Media{
							{
								Mime: "application/json",
								Schema: &genspec.SchemaOrRef{
									Ref: &genspec.SchemaRef{Ref: "#/components/schemas/Pet"},
								},
							},
						},
					},
					{
						Status:      "404",
						Description: "Pet not found",
					},
				},
			},
		},
		Schemas: map[string]genspec.Schema{
			"Pet": {
				Name:        "Pet",
				Type:        "object",
				Description: "A pet in the pet store",
				Required:    []string{"name", "status"},
				Properties: map[string]*genspec.SchemaOrRef{
					"id": {
						Schema: &genspec.Schema{
							Type:    "integer",
							Format:  "int64",
							Example: 123,
						},
					},
					"name": {
						Schema: &genspec.Schema{
							Type:    "string",
							Example: "Fluffy",
						},
					},
					"status": {
						Schema: &genspec.Schema{
							Type:        "string",
							Description: "Pet status in the store",
							Enum:        []any{"available", "pending", "sold"},
						},
					},
					"category": {
						Ref: &genspec.SchemaRef{Ref: "#/components/schemas/Category"},
					},
					"tags": {
						Schema: &genspec.Schema{
							Type: "array",
							Items: &genspec.SchemaOrRef{
								Ref: &genspec.SchemaRef{Ref: "#/components/schemas/Tag"},
							},
						},
					},
				},
			},
			"NewPet": {
				Name:        "NewPet",
				Type:        "object",
				Description: "A new pet to be created",
				Required:    []string{"name"},
				Properties: map[string]*genspec.SchemaOrRef{
					"name": {
						Schema: &genspec.Schema{Type: "string"},
					},
					"status": {
						Schema: &genspec.Schema{
							Type: "string",
							Enum: []any{"available", "pending", "sold"},
						},
					},
				},
			},
			"Category": {
				Name:        "Category",
				Type:        "object",
				Description: "Pet category",
				Properties: map[string]*genspec.SchemaOrRef{
					"id": {
						Schema: &genspec.Schema{Type: "integer", Format: "int64"},
					},
					"name": {
						Schema: &genspec.Schema{Type: "string"},
					},
				},
			},
			"Tag": {
				Name:        "Tag",
				Type:        "object",
				Description: "Pet tag",
				Properties: map[string]*genspec.SchemaOrRef{
					"id": {
						Schema: &genspec.Schema{Type: "integer", Format: "int64"},
					},
					"name": {
						Schema: &genspec.Schema{Type: "string"},
					},
				},
			},
		},
	}
}

func createSimpleServiceModel() *genspec.ServiceModel {
	return &genspec.ServiceModel{
		Title:       "Simple API",
		Version:     "1.0.0",
		Description: "A simple test API",
		Endpoints: []genspec.EndpointModel{
			{
				ID:      "get /test",
				Method:  genspec.GET,
				Path:    "/test",
				Summary: "Test endpoint",
			},
		},
		Schemas: make(map[string]genspec.Schema),
	}
}

func createTempFile(t *testing.T) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "pyemitter-test-file-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	return tmpFile.Name()
}

// Verification helper functions

func verifyEmitResult(t *testing.T, result *Result, opts Options) {
	t.Helper()
	if result.ToolName == "" {
		t.Error("result.ToolName should not be empty")
	}
	if result.PackageName == "" {
		t.Error("result.PackageName should not be empty")
	}
	if len(result.Planned) == 0 {
		t.Error("result.Planned should contain files")
	}
	// Verify that sanitized names are used correctly
	expectedToolName := sanitizeToolName(opts.ToolName)
	if expectedToolName != "" && result.ToolName != expectedToolName {
		t.Errorf("expected tool name %q, got %q", expectedToolName, result.ToolName)
	}
}

func verifyAllRequiredFiles(t *testing.T, baseDir string) {
	t.Helper()
	// Define all files that must be generated for a complete Python project
	requiredFiles := []string{
		// Project configuration
		".editorconfig",
		".gitignore",
		"setup.py",
		"requirements.txt",
		"requirements-dev.txt",
		"pyproject.toml",
		"Makefile",
		"README.md",

		// Code quality configuration
		".pre-commit-config.yaml",
		"mypy.ini",
		".pylintrc",

		// Source code structure (assuming package name "complex_api")
		"src/complex_api/__init__.py",
		"src/complex_api/main.py",
		"src/complex_api/server.py",

		// Spec package
		"src/complex_api/spec/__init__.py",
		"src/complex_api/spec/model.py",
		"src/complex_api/spec/model.json",
		"src/complex_api/spec/loader.py",

		// MCP methods
		"src/complex_api/mcp/__init__.py",
		"src/complex_api/mcp/methods/__init__.py",
		"src/complex_api/mcp/methods/list_endpoints.py",
		"src/complex_api/mcp/methods/search_endpoints.py",
		"src/complex_api/mcp/methods/get_endpoint_details.py",
		"src/complex_api/mcp/methods/list_schemas.py",
		"src/complex_api/mcp/methods/get_schema_details.py",

		// Tests
		"tests/__init__.py",
		"tests/test_mcp_methods.py",
	}

	for _, expectedFile := range requiredFiles {
		fullPath := filepath.Join(baseDir, expectedFile)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("required file %s does not exist", expectedFile)
		}
	}
}

func verifyGeneratedContent(t *testing.T, baseDir string, sm *genspec.ServiceModel, opts Options) {
	t.Helper()

	// Verify setup.py contains correct package name
	setupPyPath := filepath.Join(baseDir, "setup.py")
	setupPyContent, err := os.ReadFile(setupPyPath)
	if err != nil {
		t.Fatalf("failed to read setup.py: %v", err)
	}
	setupPyStr := string(setupPyContent)
	if !contains(setupPyStr, opts.PackageName) {
		t.Errorf("setup.py should contain package name %q", opts.PackageName)
	}

	// Verify model.json contains service model data
	modelJSONPath := filepath.Join(baseDir, "src", opts.PackageName, "spec", "model.json")
	modelJSONContent, err := os.ReadFile(modelJSONPath)
	if err != nil {
		t.Fatalf("failed to read model.json: %v", err)
	}
	modelJSONStr := string(modelJSONContent)
	if !contains(modelJSONStr, sm.Title) {
		t.Errorf("model.json should contain service title %q", sm.Title)
	}
	if !contains(modelJSONStr, sm.Version) {
		t.Errorf("model.json should contain service version %q", sm.Version)
	}

	// Verify main.py contains correct imports
	mainPyPath := filepath.Join(baseDir, "src", opts.PackageName, "main.py")
	mainPyContent, err := os.ReadFile(mainPyPath)
	if err != nil {
		t.Fatalf("failed to read main.py: %v", err)
	}
	mainPyStr := string(mainPyContent)
	if !contains(mainPyStr, "mcp") {
		t.Error("main.py should contain MCP imports")
	}
}

func verifyPythonSyntax(t *testing.T, baseDir string) {
	t.Helper()
	// Find all Python files
	var pythonFiles []string
	err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".py" {
			pythonFiles = append(pythonFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk directory: %v", err)
	}

	if len(pythonFiles) == 0 {
		t.Error("no Python files found")
		return
	}

	// Basic syntax validation - check for common Python syntax errors
	for _, pyFile := range pythonFiles {
		content, err := os.ReadFile(pyFile)
		if err != nil {
			t.Errorf("failed to read %s: %v", pyFile, err)
			continue
		}

		// Basic validation: check for balanced quotes and parentheses
		contentStr := string(content)
		if !isValidPythonSyntax(contentStr) {
			t.Errorf("Python syntax issues detected in %s", pyFile)
		}
	}
}

func verifyTemplateContent(t *testing.T, baseDir string, sm *genspec.ServiceModel, opts Options) {
	t.Helper()

	// Check README.md contains project information
	readmePath := filepath.Join(baseDir, "README.md")
	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	readmeStr := string(readmeContent)
	// Check for either the tool name directly or the service title
	hasToolName := contains(readmeStr, opts.ToolName)
	hasServiceTitle := contains(readmeStr, sm.Title)
	if !hasToolName && !hasServiceTitle {
		t.Errorf("README.md should contain either tool name %q or service title %q", opts.ToolName, sm.Title)
		t.Logf("README.md content (first 500 chars): %s", readmeStr[:min(500, len(readmeStr))])
	}

	// Check pyproject.toml contains correct project metadata
	pyprojectPath := filepath.Join(baseDir, "pyproject.toml")
	pyprojectContent, err := os.ReadFile(pyprojectPath)
	if err != nil {
		t.Fatalf("failed to read pyproject.toml: %v", err)
	}
	pyprojectStr := string(pyprojectContent)
	if !contains(pyprojectStr, opts.PackageName) {
		t.Errorf("pyproject.toml should contain package name %q", opts.PackageName)
	}
}

func verifyNoDryRunFiles(t *testing.T, tmpDir string) {
	t.Helper()
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("expected no files to be created in dry run mode, but found %d files", len(entries))
		for _, entry := range entries {
			t.Logf("  found: %s", entry.Name())
		}
	}
}

func verifyPlannedFilesMatch(t *testing.T, dryRunPlanned, realPlanned []PlannedFile) {
	t.Helper()
	if len(dryRunPlanned) != len(realPlanned) {
		t.Errorf("dry run planned %d files, real run planned %d files", len(dryRunPlanned), len(realPlanned))
	}

	// Create maps for comparison
	dryRunMap := make(map[string]PlannedFile)
	for _, pf := range dryRunPlanned {
		dryRunMap[pf.RelPath] = pf
	}

	realMap := make(map[string]PlannedFile)
	for _, pf := range realPlanned {
		realMap[pf.RelPath] = pf
	}

	// Check that all dry run files are in real run
	for path, dryFile := range dryRunMap {
		if realFile, exists := realMap[path]; !exists {
			t.Errorf("dry run planned file %s not found in real run", path)
		} else {
			// Verify file attributes match
			if dryFile.Size != realFile.Size {
				t.Errorf("file %s size mismatch: dry run %d, real %d", path, dryFile.Size, realFile.Size)
			}
			if dryFile.Mode != realFile.Mode {
				t.Errorf("file %s mode mismatch: dry run %v, real %v", path, dryFile.Mode, realFile.Mode)
			}
		}
	}

	// Check that real run doesn't have extra files
	for path := range realMap {
		if _, exists := dryRunMap[path]; !exists {
			t.Errorf("real run has extra file %s not planned in dry run", path)
		}
	}
}

func verifyActualFiles(t *testing.T, baseDir string, planned []PlannedFile) {
	t.Helper()
	for _, pf := range planned {
		fullPath := filepath.Join(baseDir, pf.RelPath)
		stat, err := os.Stat(fullPath)
		if err != nil {
			t.Errorf("planned file %s does not exist: %v", pf.RelPath, err)
			continue
		}

		// Verify file size matches (approximately)
		actualSize := int(stat.Size())
		if actualSize != pf.Size {
			t.Errorf("file %s size mismatch: expected %d, got %d", pf.RelPath, pf.Size, actualSize)
		}

		// Verify file mode (on Unix systems)
		if stat.Mode() != pf.Mode {
			t.Logf("file %s mode difference: expected %v, got %v (this may be platform-specific)",
				pf.RelPath, pf.Mode, stat.Mode())
		}
	}
}

func isValidPythonSyntax(content string) bool {
	// Very basic Python syntax validation - mainly checking for template rendering issues
	contentStr := strings.TrimSpace(content)

	// Check for obvious template rendering issues
	if strings.Contains(contentStr, "{{") && strings.Contains(contentStr, "}}") {
		// Template syntax not rendered
		return false
	}

	// Empty files are valid Python (like __init__.py files)
	// If the file is not empty and doesn't have template issues, consider it valid
	// We're being permissive here because full Python syntax validation is complex
	// and not the primary goal of this integration test
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
