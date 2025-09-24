package pyemitter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	genspec "github.com/mark3labs/swagger2mcp/internal/spec"
)

// Options controls how the Python emitter renders a project.
type Options struct {
	OutDir      string // required; target directory to write the project
	ToolName    string // tool binary name; used for project and package naming
	PackageName string // Python package name; defaults to normalized ToolName when empty
	Force       bool   // overwrite existing files
	DryRun      bool   // don't write, only plan
	Verbose     bool
}

// PlannedFile describes a file the emitter intends to write.
type PlannedFile struct {
	RelPath string
	Size    int
	Mode    os.FileMode
}

// Result returns the planned files and final resolved names.
type Result struct {
	ToolName    string
	PackageName string
	Planned     []PlannedFile
}

// Emit renders a Python MCP tool project using the provided ServiceModel.
func Emit(ctx context.Context, sm *genspec.ServiceModel, opts Options) (*Result, error) {
	_ = ctx
	if sm == nil {
		return nil, fmt.Errorf("pyemitter: nil ServiceModel")
	}
	if strings.TrimSpace(opts.OutDir) == "" {
		return nil, fmt.Errorf("pyemitter: OutDir is required")
	}

	toolName := sanitizeToolName(opts.ToolName)
	if toolName == "" {
		// derive from service title as a fallback
		toolName = deriveToolName(sm.Title)
		if toolName == "" {
			toolName = "mcp-tool"
		}
	}

	packageName := strings.TrimSpace(opts.PackageName)
	if packageName == "" {
		packageName = sanitizePackageName(toolName)
	}

	// Build file map
	files := map[string][]byte{}

	// Project configuration files
	templateData := NewTemplateData(toolName, packageName, sm)
	files[".editorconfig"] = []byte(renderTemplate(EditorconfigTemplate, templateData))
	files[".gitignore"] = []byte(renderTemplate(GitignoreTemplate, templateData))
	files["setup.py"] = []byte(renderTemplate(SetupPyTemplate, templateData))
	files["requirements.txt"] = []byte(renderTemplate(RequirementsTxtTemplate, templateData))
	files["requirements-dev.txt"] = []byte(renderTemplate(RequirementsDevTxtTemplate, templateData))
	files["pyproject.toml"] = []byte(renderTemplate(PyprojectTomlTemplate, templateData))
	files["Makefile"] = []byte(renderTemplate(MakefileTemplate, templateData))
	files["README.md"] = []byte(renderTemplate(ReadmeMdTemplate, templateData))

	// Code quality and development configuration files
	files[".pre-commit-config.yaml"] = []byte(renderTemplate(PreCommitConfigTemplate, templateData))
	files["mypy.ini"] = []byte(renderTemplate(MyPyConfigTemplate, templateData))
	files[".pylintrc"] = []byte(renderTemplate(PylintRcTemplate, templateData))

	// Source code structure
	srcPath := filepath.Join("src", packageName)
	files[filepath.Join(srcPath, "__init__.py")] = []byte(`"""Generated MCP tool package."""
__version__ = "0.1.0"
`)
	files[filepath.Join(srcPath, "main.py")] = []byte(renderTemplate(MainPyTemplate, templateData))
	files[filepath.Join(srcPath, "server.py")] = []byte(renderTemplate(ServerPyTemplate, templateData))

	// Spec package
	specPath := filepath.Join(srcPath, "spec")
	files[filepath.Join(specPath, "__init__.py")] = []byte("")
	files[filepath.Join(specPath, "model.py")] = []byte(renderModelPy())

	// model.json
	modelJSON, err := json.MarshalIndent(sm, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal model.json: %w", err)
	}
	files[filepath.Join(specPath, "model.json")] = append(modelJSON, '\n')
	files[filepath.Join(specPath, "loader.py")] = []byte(renderLoaderPy())

	// MCP methods
	mcpPath := filepath.Join(srcPath, "mcp")
	files[filepath.Join(mcpPath, "__init__.py")] = []byte("")
	methodsPath := filepath.Join(mcpPath, "methods")
	files[filepath.Join(methodsPath, "__init__.py")] = []byte(renderTemplate(MethodsInitPyTemplate, templateData))
	files[filepath.Join(methodsPath, "list_endpoints.py")] = []byte(renderTemplate(ListEndpointsPyTemplate, templateData))
	files[filepath.Join(methodsPath, "search_endpoints.py")] = []byte(renderTemplate(SearchEndpointsPyTemplate, templateData))
	files[filepath.Join(methodsPath, "get_endpoint_details.py")] = []byte(renderTemplate(GetEndpointDetailsPyTemplate, templateData))
	files[filepath.Join(methodsPath, "list_schemas.py")] = []byte(renderTemplate(ListSchemasPyTemplate, templateData))
	files[filepath.Join(methodsPath, "get_schema_details.py")] = []byte(renderTemplate(GetSchemaDetailsPyTemplate, templateData))

	// Tests
	testsPath := "tests"
	files[filepath.Join(testsPath, "__init__.py")] = []byte(renderTemplate(TestsInitPyTemplate, templateData))
	files[filepath.Join(testsPath, "test_mcp_methods.py")] = []byte(renderTemplate(TestMCPMethodsPyTemplate, templateData))

	// Plan in deterministic order
	rels := make([]string, 0, len(files))
	for p := range files {
		rels = append(rels, filepath.ToSlash(p))
	}
	sort.Strings(rels)

	planned := make([]PlannedFile, 0, len(rels))
	for _, rel := range rels {
		// Determine appropriate file mode
		var fileMode os.FileMode = 0o644
		if isExecutable(rel) {
			fileMode = 0o755
		}
		planned = append(planned, PlannedFile{
			RelPath: rel,
			Size:    len(files[rel]),
			Mode:    fileMode,
		})
	}

	// Write files if not in dry-run mode
	if !opts.DryRun {
		if err := writeFiles(opts.OutDir, files, opts.Force); err != nil {
			return nil, err
		}
	} else {
		// In dry-run mode, validate output directory without writing
		abs, err := filepath.Abs(opts.OutDir)
		if err != nil {
			return nil, fmt.Errorf("pyemitter: resolve output directory: %w", err)
		}
		if err := validateOutputDirectory(abs, opts.Force); err != nil {
			return nil, err
		}
	}

	return &Result{ToolName: toolName, PackageName: packageName, Planned: planned}, nil
}

func writeFiles(outDir string, files map[string][]byte, force bool) error {
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("pyemitter: resolve output directory: %w", err)
	}

	// Pre-flight: validate output directory and force flag
	if err := validateOutputDirectory(abs, force); err != nil {
		return err
	}

	// Create directory structure first (all parent directories)
	if err := createDirectoryStructure(abs, files); err != nil {
		return fmt.Errorf("pyemitter: create directory structure: %w", err)
	}

	// Write files atomically with proper permissions
	for rel, content := range files {
		if err := writeFileAtomic(abs, rel, content); err != nil {
			return fmt.Errorf("pyemitter: write file %s: %w", rel, err)
		}
	}

	return nil
}

// validateOutputDirectory checks if the output directory is valid for writing
func validateOutputDirectory(absPath string, force bool) error {
	stat, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		// Directory doesn't exist, will be created - this is fine
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot access output directory %q: %w", absPath, err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("output path %q is not a directory", absPath)
	}

	// If force flag is set, we can proceed regardless of content
	if force {
		return nil
	}

	// Check if directory is empty
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return fmt.Errorf("cannot read output directory %q: %w", absPath, err)
	}

	if len(entries) > 0 {
		return fmt.Errorf("output directory %q is not empty (use --force to overwrite)", absPath)
	}

	return nil
}

// createDirectoryStructure creates all necessary directories for the file structure
func createDirectoryStructure(baseDir string, files map[string][]byte) error {
	dirsToCreate := make(map[string]bool)

	// Collect all unique directory paths
	for relPath := range files {
		dir := filepath.Dir(relPath)
		if dir != "." {
			// Normalize path separators and collect all parent directories
			normalizedDir := filepath.ToSlash(dir)
			parts := strings.Split(normalizedDir, "/")
			currentPath := ""
			for _, part := range parts {
				if currentPath == "" {
					currentPath = part
				} else {
					currentPath = currentPath + "/" + part
				}
				dirsToCreate[currentPath] = true
			}
		}
	}

	// Create directories with proper permissions
	for dirPath := range dirsToCreate {
		fullPath := filepath.Join(baseDir, dirPath)
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dirPath, err)
		}
	}

	return nil
}

// writeFileAtomic writes a file atomically using temporary file + rename
func writeFileAtomic(baseDir, relPath string, content []byte) error {
	fullPath := filepath.Join(baseDir, relPath)

	// Determine file mode based on file type
	var fileMode os.FileMode = 0o644
	if isExecutable(relPath) {
		fileMode = 0o755
	}

	// Ensure target directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure target directory %s: %w", dir, err)
	}

	// Create temporary file in the same directory as the target
	tmpFile, err := os.CreateTemp(dir, ".tmp-pyemitter-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", relPath, err)
	}

	tmpPath := tmpFile.Name()
	success := false

	// Ensure cleanup on error
	defer func() {
		if tmpFile != nil {
			tmpFile.Close()
		}
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Write content to temporary file with buffered I/O
	if len(content) > 0 {
		n, err := tmpFile.Write(content)
		if err != nil {
			return fmt.Errorf("write content to temp file: %w", err)
		}
		if n != len(content) {
			return fmt.Errorf("incomplete write: expected %d bytes, wrote %d", len(content), n)
		}
	}

	// Sync to ensure data is written to disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	// Set file permissions before closing
	if err := tmpFile.Chmod(fileMode); err != nil {
		return fmt.Errorf("set file permissions: %w", err)
	}

	// Close temp file to ensure all data is written
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	tmpFile = nil

	// Atomically move temp file to final location
	if err := os.Rename(tmpPath, fullPath); err != nil {
		return fmt.Errorf("atomic rename %s to %s: %w", tmpPath, fullPath, err)
	}

	// Mark as successfully processed
	success = true
	return nil
}

// isExecutable determines if a file should have executable permissions
func isExecutable(relPath string) bool {
	// Python entry point scripts and Makefiles should be executable
	fileName := filepath.Base(relPath)
	ext := filepath.Ext(relPath)

	// Makefiles should be executable
	if fileName == "Makefile" || fileName == "makefile" {
		return true
	}

	// Python main entry scripts should be executable
	if strings.HasSuffix(relPath, "/main.py") || strings.HasSuffix(relPath, "\\main.py") {
		return true
	}

	// Shell scripts should be executable
	if ext == ".sh" || ext == ".bash" {
		return true
	}

	return false
}

func sanitizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// replace spaces and slashes
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ToLower(name)
	// keep alnum, dash, underscore only
	b := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	return out
}

func sanitizePackageName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	// Python package names: replace dashes with underscores, keep lowercase
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ToLower(name)
	// keep alnum and underscore only
	b := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	out = strings.Trim(out, "_")
	return out
}

func deriveToolName(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return ""
	}
	// split on spaces and punctuation, join with dash
	t = strings.ToLower(t)
	repl := strings.NewReplacer("/", " ", "_", " ", ".", " ", ",", " ", ":", " ")
	t = repl.Replace(t)
	parts := strings.Fields(t)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "-")
}

// renderTemplate renders a template string using the provided template data
func renderTemplate(templateContent string, data TemplateData) string {
	result, err := RenderTemplateWithErrorHandling("template", templateContent, data)
	if err != nil {
		// Return a fallback with error message
		return fmt.Sprintf("# Error rendering template: %v\n", err)
	}
	return result
}

// renderModelPy renders the model.py file (specific implementation)
func renderModelPy() string {
	templateData := TemplateData{
		ToolName:     "model",
		PackageName:  "model",
		ServiceTitle: "Data Model",
		ServiceModel: nil, // Not needed for model template
		Version:      "0.1.0",
		Author:       "Generated by swagger2mcp",
	}

	// Use a basic model template since the full one is complex
	template := `"""Data model definitions for MCP server.

This module defines the core data structures used by the MCP server
to represent API documentation and schema information. These structures
are compatible with the Go ServiceModel definitions.

Generated by swagger2mcp - DO NOT MODIFY MANUALLY
"""

from __future__ import annotations
from dataclasses import dataclass, field
from typing import List, Dict, Optional, Any, Union
from enum import Enum
import json


class HttpMethod(str, Enum):
    """HTTP method enumeration matching OpenAPI specification."""
    GET = "get"
    POST = "post"
    PUT = "put"
    DELETE = "delete"
    PATCH = "patch"
    HEAD = "head"
    OPTIONS = "options"
    TRACE = "trace"

    def __str__(self) -> str:
        return self.value


@dataclass
class Server:
    """Server information from OpenAPI specification."""
    url: str = ""
    description: str = ""


@dataclass
class SchemaRef:
    """Schema reference containing a $ref pointer."""
    ref: str = ""


@dataclass
class SchemaOrRef:
    """Container for either a Schema or a SchemaRef."""
    schema: Optional["Schema"] = None
    ref: Optional[SchemaRef] = None


@dataclass
class Schema:
    """Schema definition from OpenAPI specification."""
    name: str = ""
    type: str = ""
    properties: Optional[Dict[str, SchemaOrRef]] = None
    required: Optional[List[str]] = None
    items: Optional[SchemaOrRef] = None
    all_of: Optional[List[SchemaOrRef]] = None
    any_of: Optional[List[SchemaOrRef]] = None
    one_of: Optional[List[SchemaOrRef]] = None
    description: str = ""
    enum: Optional[List[Any]] = None
    format: Optional[str] = None
    example: Any = None


@dataclass
class Media:
    """Media type definition for request/response content."""
    mime: str = ""
    schema: Optional[SchemaOrRef] = None
    example: Any = None


@dataclass
class ParameterModel:
    """Parameter definition for API endpoints."""
    name: str = ""
    in_: str = ""  # path|query|header|cookie
    required: bool = False
    schema: Optional[SchemaOrRef] = None


@dataclass
class RequestBodyModel:
    """Request body definition for API endpoints."""
    content: List[Media] = field(default_factory=list)
    required: bool = False


@dataclass
class ResponseModel:
    """Response definition for API endpoints."""
    status: str = ""  # 200, 4xx, default
    description: str = ""
    content: List[Media] = field(default_factory=list)


@dataclass
class EndpointModel:
    """API endpoint definition from OpenAPI specification."""
    id: str = ""  # method+path
    method: HttpMethod = HttpMethod.GET
    path: str = ""
    summary: str = ""
    description: str = ""
    tags: List[str] = field(default_factory=list)
    parameters: List[ParameterModel] = field(default_factory=list)
    request_body: Optional[RequestBodyModel] = None
    responses: List[ResponseModel] = field(default_factory=list)


@dataclass
class ServiceModel:
    """Root service model containing all API documentation."""
    title: str = ""
    version: str = ""
    description: str = ""
    servers: List[Server] = field(default_factory=list)
    tags: List[str] = field(default_factory=list)
    endpoints: List[EndpointModel] = field(default_factory=list)
    schemas: Dict[str, Schema] = field(default_factory=dict)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "ServiceModel":
        """Create ServiceModel from dictionary with complete parsing."""
        # Parse servers
        servers = []
        servers_data = data.get("Servers", data.get("servers", []))
        if servers_data:
            for server_data in servers_data:
                if server_data:  # Check for None
                    servers.append(Server(
                        url=server_data.get("URL", server_data.get("url", "")),
                        description=server_data.get("Description", server_data.get("description", ""))
                    ))
        
        # Parse endpoints
        endpoints = []
        endpoints_data = data.get("Endpoints", data.get("endpoints", []))
        if endpoints_data:
            for endpoint_data in endpoints_data:
                if not endpoint_data:  # Skip None endpoints
                    continue
                    
                # Parse parameters
                parameters = []
                parameters_data = endpoint_data.get("Parameters", endpoint_data.get("parameters", []))
                if parameters_data:
                    for param_data in parameters_data:
                        if not param_data:  # Skip None parameters
                            continue
                            
                        schema = None
                        if "Schema" in param_data or "schema" in param_data:
                            schema_data = param_data.get("Schema", param_data.get("schema"))
                            if schema_data and "$ref" in schema_data:
                                schema = SchemaOrRef(ref=SchemaRef(ref=schema_data["$ref"]))
                            elif schema_data:
                                schema = SchemaOrRef(schema=Schema(
                                    type=schema_data.get("Type", schema_data.get("type", "")),
                                    format=schema_data.get("Format", schema_data.get("format"))
                                ))
                        
                        parameters.append(ParameterModel(
                            name=param_data.get("Name", param_data.get("name", "")),
                            in_=param_data.get("In", param_data.get("in", "")),
                            required=param_data.get("Required", param_data.get("required", False)),
                            schema=schema
                        ))
            
                # Parse request body
                request_body = None
                if "RequestBody" in endpoint_data or "requestBody" in endpoint_data:
                    rb_data = endpoint_data.get("RequestBody", endpoint_data.get("requestBody"))
                    if rb_data:
                        content = []
                        content_data = rb_data.get("Content", rb_data.get("content", []))
                        if content_data:
                            for media_data in content_data:
                                if not media_data:  # Skip None media
                                    continue
                                    
                                schema = None
                                if "Schema" in media_data or "schema" in media_data:
                                    schema_data = media_data.get("Schema", media_data.get("schema"))
                                    if schema_data and "$ref" in schema_data:
                                        schema = SchemaOrRef(ref=SchemaRef(ref=schema_data["$ref"]))
                                    elif schema_data:
                                        schema = SchemaOrRef(schema=Schema(
                                            type=schema_data.get("Type", schema_data.get("type", "")),
                                            format=schema_data.get("Format", schema_data.get("format"))
                                        ))
                                
                                content.append(Media(
                                    mime=media_data.get("Mime", media_data.get("mime", "")),
                                    schema=schema
                                ))
                        
                        request_body = RequestBodyModel(
                            content=content,
                            required=rb_data.get("Required", rb_data.get("required", False))
                        )
            
                # Parse responses
                responses = []
                responses_data = endpoint_data.get("Responses", endpoint_data.get("responses", []))
                if responses_data:
                    for resp_data in responses_data:
                        if not resp_data:  # Skip None responses
                            continue
                            
                        content = []
                        content_data = resp_data.get("Content", resp_data.get("content", []))
                        if content_data:
                            for media_data in content_data:
                                if not media_data:  # Skip None media
                                    continue
                                    
                                schema = None
                                if "Schema" in media_data or "schema" in media_data:
                                    schema_data = media_data.get("Schema", media_data.get("schema"))
                                    if schema_data and "$ref" in schema_data:
                                        schema = SchemaOrRef(ref=SchemaRef(ref=schema_data["$ref"]))
                                    elif schema_data:
                                        schema = SchemaOrRef(schema=Schema(
                                            type=schema_data.get("Type", schema_data.get("type", "")),
                                            format=schema_data.get("Format", schema_data.get("format"))
                                        ))
                                
                                content.append(Media(
                                    mime=media_data.get("Mime", media_data.get("mime", "")),
                                    schema=schema
                                ))
                        
                        responses.append(ResponseModel(
                            status=resp_data.get("Status", resp_data.get("status", "")),
                            description=resp_data.get("Description", resp_data.get("description", "")),
                            content=content
                        ))
            
                endpoints.append(EndpointModel(
                    id=endpoint_data.get("ID", endpoint_data.get("id", "")),
                    method=HttpMethod(endpoint_data.get("Method", endpoint_data.get("method", "get"))),
                    path=endpoint_data.get("Path", endpoint_data.get("path", "")),
                    summary=endpoint_data.get("Summary", endpoint_data.get("summary", "")),
                    description=endpoint_data.get("Description", endpoint_data.get("description", "")),
                    tags=endpoint_data.get("Tags", endpoint_data.get("tags", [])),
                    parameters=parameters,
                    request_body=request_body,
                    responses=responses
                ))
        
        # Parse schemas
        schemas = {}
        schemas_data = data.get("Schemas", data.get("schemas", {}))
        if schemas_data:
            for schema_name, schema_data in schemas_data.items():
                if not schema_data:  # Skip None schemas
                    continue
                    
                # Parse properties
                properties = {}
                if "Properties" in schema_data or "properties" in schema_data:
                    props_data = schema_data.get("Properties", schema_data.get("properties", {}))
                    if props_data:
                        for prop_name, prop_data in props_data.items():
                            if prop_data and "$ref" in prop_data:
                                properties[prop_name] = SchemaOrRef(ref=SchemaRef(ref=prop_data["$ref"]))
                            elif prop_data:
                                properties[prop_name] = SchemaOrRef(schema=Schema(
                                    type=prop_data.get("Type", prop_data.get("type", "")),
                                    format=prop_data.get("Format", prop_data.get("format"))
                                ))
                
                schemas[schema_name] = Schema(
                    name=schema_name,
                    type=schema_data.get("Type", schema_data.get("type", "")),
                    properties=properties,
                    required=schema_data.get("Required", schema_data.get("required", [])),
                    description=schema_data.get("Description", schema_data.get("description", ""))
                )
        
        return cls(
            title=data.get("title", data.get("Title", "")),
            version=data.get("version", data.get("Version", "")),
            description=data.get("description", data.get("Description", "")),
            servers=servers,
            tags=data.get("tags", data.get("Tags", [])),
            endpoints=endpoints,
            schemas=schemas
        )
`

	result, err := RenderTemplateWithErrorHandling("model.py", template, templateData)
	if err != nil {
		return fmt.Sprintf("# Error rendering model.py: %v", err)
	}
	return result
}

// renderLoaderPy renders the loader.py file (specific implementation)
func renderLoaderPy() string {
	template := `"""Service model loader for MCP server.

This module handles loading the embedded service model from the JSON file
and creating Python data structure instances.

Generated by swagger2mcp - DO NOT MODIFY MANUALLY
"""

import json
import os
import logging
from pathlib import Path
from typing import Optional

from .model import ServiceModel

logger = logging.getLogger(__name__)


class ServiceModelLoadError(Exception):
    """Exception raised when service model loading fails."""
    pass


def load_service_model() -> ServiceModel:
    """Load the embedded service model from model.json.
    
    Returns:
        ServiceModel: The loaded and validated service model instance.
        
    Raises:
        ServiceModelLoadError: If the model cannot be loaded or is invalid.
    """
    try:
        # Get the path to model.json
        current_dir = Path(__file__).parent
        model_json_path = current_dir / "model.json"
        
        if not model_json_path.exists():
            raise ServiceModelLoadError(f"Model file not found: {model_json_path}")
        
        # Load and parse JSON
        with open(model_json_path, 'r', encoding='utf-8') as f:
            json_data = json.load(f)
        
        # Create ServiceModel instance
        service_model = ServiceModel.from_dict(json_data)
        
        logger.info(f"Successfully loaded service model: {service_model.title} v{service_model.version}")
        return service_model
        
    except Exception as e:
        raise ServiceModelLoadError(f"Failed to load service model: {e}") from e


def load_service_model_safe() -> Optional[ServiceModel]:
    """Load the service model with exception handling.
    
    Returns:
        ServiceModel: The loaded service model, or None if loading failed.
    """
    try:
        return load_service_model()
    except ServiceModelLoadError as e:
        logger.error(f"Failed to load service model: {e}")
        return None


__all__ = ["ServiceModelLoadError", "load_service_model", "load_service_model_safe"]
`

	return template
}
