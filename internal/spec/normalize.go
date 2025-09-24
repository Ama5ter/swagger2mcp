package spec

import (
    "context"
    "fmt"
    "regexp"
    "sort"
    "strings"
    "sync"

    "github.com/getkin/kin-openapi/openapi3"
    "gopkg.in/yaml.v3"
)

// v2SchemaStorage holds original v2 schema definitions and cached operations for documents that need enhanced parsing
var v2SchemaStorage = struct {
    mu    sync.RWMutex
    store map[*openapi3.T]struct {
        definitions map[string]any                  // v2 definitions
        rawBytes    []byte                          // original v2 raw bytes
        operations  map[string]map[string]any       // cached v2 operations by path/method
    }
}{
    store: make(map[*openapi3.T]struct {
        definitions map[string]any
        rawBytes    []byte
        operations  map[string]map[string]any
    }),
}

// SetV2SchemaDefinitions stores v2 schema definitions and raw bytes for a document
func SetV2SchemaDefinitions(doc *openapi3.T, v2Raw []byte) {
    if doc == nil || v2Raw == nil {
        return
    }
    
    definitions := extractV2Schemas(v2Raw)
    if definitions == nil {
        return
    }
    
    // Pre-parse operations for better performance
    operations := extractV2Operations(v2Raw)
    
    v2SchemaStorage.mu.Lock()
    defer v2SchemaStorage.mu.Unlock()
    v2SchemaStorage.store[doc] = struct {
        definitions map[string]any
        rawBytes    []byte
        operations  map[string]map[string]any
    }{
        definitions: definitions,
        rawBytes:    v2Raw,
        operations:  operations,
    }
}

// getV2SchemaDefinitions retrieves v2 schema definitions for a document
func getV2SchemaDefinitions(doc *openapi3.T) map[string]any {
    if doc == nil {
        return nil
    }
    
    v2SchemaStorage.mu.RLock()
    defer v2SchemaStorage.mu.RUnlock()
    if data, exists := v2SchemaStorage.store[doc]; exists {
        return data.definitions
    }
    return nil
}

// getV2Operations retrieves cached v2 operations for a document
func getV2Operations(doc *openapi3.T) map[string]map[string]any {
    if doc == nil {
        return nil
    }
    
    v2SchemaStorage.mu.RLock()
    defer v2SchemaStorage.mu.RUnlock()
    if data, exists := v2SchemaStorage.store[doc]; exists {
        return data.operations
    }
    return nil
}

// BuildOption configures how the ServiceModel is built from an OpenAPI doc.
type BuildOption func(*buildConfig)

type buildConfig struct {
    includeTags map[string]struct{}
    excludeTags map[string]struct{}
    methods     map[HttpMethod]struct{}
    pathRes     []*regexp.Regexp
}

// WithIncludeTags keeps only endpoints that have at least one of the given tags.
func WithIncludeTags(tags []string) BuildOption {
    return func(c *buildConfig) {
        if len(tags) == 0 {
            return
        }
        if c.includeTags == nil {
            c.includeTags = make(map[string]struct{}, len(tags))
        }
        for _, t := range tags {
            t = strings.TrimSpace(t)
            if t == "" {
                continue
            }
            c.includeTags[t] = struct{}{}
        }
    }
}

// WithExcludeTags removes endpoints that have any of the given tags.
func WithExcludeTags(tags []string) BuildOption {
    return func(c *buildConfig) {
        if len(tags) == 0 {
            return
        }
        if c.excludeTags == nil {
            c.excludeTags = make(map[string]struct{}, len(tags))
        }
        for _, t := range tags {
            t = strings.TrimSpace(t)
            if t == "" {
                continue
            }
            c.excludeTags[t] = struct{}{}
        }
    }
}

// WithMethods keeps only endpoints using one of the provided HTTP methods.
func WithMethods(methods []HttpMethod) BuildOption {
    return func(c *buildConfig) {
        if len(methods) == 0 {
            return
        }
        if c.methods == nil {
            c.methods = make(map[HttpMethod]struct{}, len(methods))
        }
        for _, m := range methods {
            c.methods[m] = struct{}{}
        }
    }
}

// WithPathPatterns keeps only endpoints whose path matches at least one of the provided
// regular expressions. Patterns are treated as regular expressions.
func WithPathPatterns(patterns []string) BuildOption {
    return func(c *buildConfig) {
        for _, p := range patterns {
            p = strings.TrimSpace(p)
            if p == "" {
                continue
            }
            re, err := regexp.Compile(p)
            if err != nil {
                // Be strict and surface invalid patterns to the caller by panicking here
                // would be unfriendly. Instead, store a sentinel that never matches.
                re = regexp.MustCompile("a^$")
            }
            c.pathRes = append(c.pathRes, re)
        }
    }
}

// BuildServiceModel converts an OpenAPI v3 document into the Internal Model (IM).
// It applies include/exclude tag filtering and optional method/path filters.
// If the v2Raw parameter is provided, it will be used to extract detailed schema
// information for Swagger 2.0 specs where the conversion may have lost schema details.
func BuildServiceModel(ctx context.Context, doc *openapi3.T, v2Raw []byte, opts ...BuildOption) (*ServiceModel, error) {
    _ = ctx
    if doc == nil {
        return nil, fmt.Errorf("nil document")
    }

    cfg := &buildConfig{}
    for _, opt := range opts {
        opt(cfg)
    }

    sm := &ServiceModel{
        Title:       safeStr(doc.Info.Title),
        Version:     safeStr(doc.Info.Version),
        Description: safeStr(doc.Info.Description),
    }

    // Servers
    if doc.Servers != nil {
        for _, s := range doc.Servers {
            if s == nil {
                continue
            }
            sm.Servers = append(sm.Servers, Server{URL: safeStr(s.URL), Description: safeStr(s.Description)})
        }
    }

    // Schemas
    if doc.Components != nil && doc.Components.Schemas != nil {
        sm.Schemas = make(map[string]Schema, len(doc.Components.Schemas))
        
        // Parse v2 schemas if available for more detailed information
        var v2Schemas map[string]any
        if v2Raw != nil {
            v2Schemas = extractV2Schemas(v2Raw)
        } else {
            // Try to get v2 schemas from global storage
            v2Schemas = getV2SchemaDefinitions(doc)
        }
        
        // Deterministic order is not required for maps, but we sort keys to build consistently when needed.
        keys := make([]string, 0, len(doc.Components.Schemas))
        for name := range doc.Components.Schemas {
            keys = append(keys, name)
        }
        sort.Strings(keys)
        for _, name := range keys {
            ref := doc.Components.Schemas[name]
            if ref == nil {
                continue
            }
            
            // Try to get detailed schema from v2 definitions if available
            var sor *SchemaOrRef
            if v2Schemas != nil {
                if v2Schema, found := v2Schemas[name]; found {
                    sor = toSchemaOrRefFromV2(v2Schema, name)
                }
            }
            
            // Fallback to converted v3 schema if v2 parsing failed
            if sor == nil {
                sor = toSchemaOrRef(ref)
            }
            
            if sor == nil {
                continue
            }
            // For top-level entries, if it's a $ref, preserve ref. If concrete, set Name.
            if sor.Ref != nil {
                // Store a lightweight placeholder Schema carrying just the ref as the name.
                sm.Schemas[name] = Schema{Name: name}
                continue
            }
            schema := *sor.Schema
            schema.Name = name
            sm.Schemas[name] = schema
        }
    }

    // Paths and operations
    if doc.Paths != nil {
        // Sort paths for determinism
        pathKeys := make([]string, 0, len(doc.Paths))
        for p := range doc.Paths {
            pathKeys = append(pathKeys, p)
        }
        sort.Strings(pathKeys)

        for _, p := range pathKeys {
            item := doc.Paths[p]
            if item == nil {
                continue
            }
            // Merge parameters: path-level first, overridden by op-level.
            baseParams := make(map[string]*ParameterModel)
            for _, pref := range item.Parameters {
                pm := toParameterModel(pref)
                if pm == nil {
                    continue
                }
                baseParams[paramKey(pm.In, pm.Name)] = pm
            }

            // Supported HTTP methods in a stable order
            ops := []struct {
                m HttpMethod
                o *openapi3.Operation
            }{
                {GET, item.Get},
                {POST, item.Post},
                {PUT, item.Put},
                {DELETE, item.Delete},
                {PATCH, item.Patch},
                {HEAD, item.Head},
                {OPTIONS, item.Options},
                {TRACE, item.Trace},
            }

            for _, pair := range ops {
                if pair.o == nil {
                    continue
                }
                // Method filter
                if len(cfg.methods) > 0 {
                    if _, ok := cfg.methods[pair.m]; !ok {
                        continue
                    }
                }
                // Path pattern filter
                if len(cfg.pathRes) > 0 {
                    matched := false
                    for _, re := range cfg.pathRes {
                        if re.MatchString(p) {
                            matched = true
                            break
                        }
                    }
                    if !matched {
                        continue
                    }
                }

                // Merge parameters with precedence to operation-level ones.
                mergedParams := make(map[string]*ParameterModel, len(baseParams))
                for k, v := range baseParams {
                    mergedParams[k] = v
                }
                for _, pref := range pair.o.Parameters {
                    pm := toParameterModel(pref)
                    if pm == nil {
                        continue
                    }
                    mergedParams[paramKey(pm.In, pm.Name)] = pm
                }
                // Materialize and sort parameters
                params := make([]ParameterModel, 0, len(mergedParams))
                for _, v := range mergedParams {
                    params = append(params, *v)
                }
                sort.Slice(params, func(i, j int) bool {
                    if params[i].In == params[j].In {
                        return params[i].Name < params[j].Name
                    }
                    return params[i].In < params[j].In
                })

                // Request body
                var rb *RequestBodyModel
                if pair.o.RequestBody != nil && pair.o.RequestBody.Value != nil {
                    rb = &RequestBodyModel{Required: pair.o.RequestBody.Value.Required}
                    
                    // Try to enhance with cached v2 operations
                    v2Ops := getV2Operations(doc)
                    if v2Ops != nil {
                        rb.Content = toMediaListWithV2Cache(pair.o.RequestBody.Value.Content, v2Ops, p, string(pair.m))
                    } else {
                        rb.Content = toMediaList(pair.o.RequestBody.Value.Content)
                    }
                }

                // Responses
                var responses []ResponseModel
                if pair.o.Responses != nil {
                    // In kin-openapi v0.116, Responses is a map[string]*ResponseRef
                    keys := make([]string, 0, len(pair.o.Responses))
                    for k := range pair.o.Responses {
                        keys = append(keys, k)
                    }
                    sort.Strings(keys)
                    for _, code := range keys {
                        rref := pair.o.Responses[code]
                        if rref == nil || rref.Value == nil {
                            continue
                        }
                        desc := ""
                        if rref.Value.Description != nil {
                            desc = *rref.Value.Description
                        }
                        
                        // Try to enhance with cached v2 operations  
                        var content []Media
                        v2Ops := getV2Operations(doc)
                        if v2Ops != nil {
                            content = toMediaListWithV2Cache(rref.Value.Content, v2Ops, p, string(pair.m))
                        } else {
                            content = toMediaList(rref.Value.Content)
                        }
                        
                        responses = append(responses, ResponseModel{
                            Status:      code,
                            Description: desc,
                            Content:     content,
                        })
                    }
                }

                // Tags and filtering
                tags := make([]string, 0, len(pair.o.Tags))
                for _, t := range pair.o.Tags {
                    t = strings.TrimSpace(t)
                    if t != "" {
                        tags = append(tags, t)
                    }
                }
                if !allowByTags(tags, cfg) {
                    continue
                }

                ep := EndpointModel{
                    ID:          string(pair.m) + " " + p,
                    Method:      pair.m,
                    Path:        p,
                    Summary:     safeStr(pair.o.Summary),
                    Description: safeStr(pair.o.Description),
                    Tags:        tags,
                    Parameters:  params,
                    RequestBody: rb,
                    Responses:   responses,
                }

                sm.Endpoints = append(sm.Endpoints, ep)
            }
        }
    }

    // Collect tags present in included endpoints
    sm.Tags = collectSortedTags(sm.Endpoints)

    return sm, nil
}

func allowByTags(tags []string, cfg *buildConfig) bool {
    hasInclude := len(cfg.includeTags) > 0
    if hasInclude {
        ok := false
        for _, t := range tags {
            if _, yes := cfg.includeTags[t]; yes {
                ok = true
                break
            }
        }
        if !ok {
            return false
        }
    }
    if len(cfg.excludeTags) > 0 {
        for _, t := range tags {
            if _, blocked := cfg.excludeTags[t]; blocked {
                return false
            }
        }
    }
    return true
}

func paramKey(in, name string) string { return in + ":" + name }

func safeStr(s string) string { return strings.TrimSpace(s) }

func toParameterModel(pref *openapi3.ParameterRef) *ParameterModel {
    if pref == nil || pref.Value == nil {
        return nil
    }
    p := pref.Value
    pm := &ParameterModel{
        Name:     safeStr(p.Name),
        In:       safeStr(p.In),
        Required: p.Required,
    }
    if p.Schema != nil {
        pm.Schema = toSchemaOrRef(p.Schema)
    }
    return pm
}

func toMediaList(content openapi3.Content) []Media {
    if content == nil {
        return nil
    }
    keys := make([]string, 0, len(content))
    for k := range content {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    out := make([]Media, 0, len(keys))
    for _, mime := range keys {
        mt := content[mime]
        if mt == nil {
            continue
        }
        var ex any
        if mt.Example != nil {
            ex = mt.Example
        } else if len(mt.Examples) > 0 {
            // Pick the first example value deterministically by key
            enames := make([]string, 0, len(mt.Examples))
            for name := range mt.Examples {
                enames = append(enames, name)
            }
            sort.Strings(enames)
            if ref := mt.Examples[enames[0]]; ref != nil && ref.Value != nil {
                ex = ref.Value.Value
            }
        }
        out = append(out, Media{
            Mime:   mime,
            Schema: toSchemaOrRef(mt.Schema),
            Example: ex,
        })
    }
    if len(out) == 0 {
        return nil
    }
    return out
}

func toSchemaOrRef(ref *openapi3.SchemaRef) *SchemaOrRef {
    if ref == nil {
        return nil
    }
    if ref.Ref != "" {
        return &SchemaOrRef{Ref: &SchemaRef{Ref: ref.Ref}}
    }
    if ref.Value == nil {
        // For components.schemas entries, the ref.Value might be nil even though content exists
        // This seems to be an issue with how kin-openapi handles converted Swagger 2.0 schemas
        // Let's create a basic placeholder schema with just a name
        if ref.Ref == "" {
            // This is a top-level schema definition without content, create a placeholder
            return &SchemaOrRef{
                Schema: &Schema{
                    Type: "object", // Default assumption for top-level schemas
                },
            }
        }
        return nil
    }
    s := &Schema{
        // In v0.116, Schema.Type is string; but guard in case of zero value
        Type:        safeStr(ref.Value.Type),
        Description: safeStr(ref.Value.Description),
        Format:      safeStr(ref.Value.Format),
        Example:     ref.Value.Example,
        Required:    append([]string(nil), ref.Value.Required...),
    }
    // Enum values
    if len(ref.Value.Enum) > 0 {
        s.Enum = append([]any(nil), ref.Value.Enum...)
    }
    // Items
    if ref.Value.Items != nil {
        s.Items = toSchemaOrRef(ref.Value.Items)
    }
    // Properties
    if len(ref.Value.Properties) > 0 {
        s.Properties = make(map[string]*SchemaOrRef, len(ref.Value.Properties))
        // Deterministic: iterate keys in order
        keys := make([]string, 0, len(ref.Value.Properties))
        for name := range ref.Value.Properties {
            keys = append(keys, name)
        }
        sort.Strings(keys)
        for _, name := range keys {
            s.Properties[name] = toSchemaOrRef(ref.Value.Properties[name])
        }
    }
    // Compositions
    if len(ref.Value.AllOf) > 0 {
        for _, r := range ref.Value.AllOf {
            s.AllOf = append(s.AllOf, toSchemaOrRef(r))
        }
    }
    if len(ref.Value.AnyOf) > 0 {
        for _, r := range ref.Value.AnyOf {
            s.AnyOf = append(s.AnyOf, toSchemaOrRef(r))
        }
    }
    if len(ref.Value.OneOf) > 0 {
        for _, r := range ref.Value.OneOf {
            s.OneOf = append(s.OneOf, toSchemaOrRef(r))
        }
    }
    return &SchemaOrRef{Schema: s}
}

func collectSortedTags(endpoints []EndpointModel) []string {
    set := make(map[string]struct{})
    for _, ep := range endpoints {
        for _, t := range ep.Tags {
            if t = strings.TrimSpace(t); t != "" {
                set[t] = struct{}{}
            }
        }
    }
    if len(set) == 0 {
        return nil
    }
    out := make([]string, 0, len(set))
    for t := range set {
        out = append(out, t)
    }
    sort.Strings(out)
    return out
}

// extractV2Schemas parses the Swagger v2.0 raw YAML/JSON to extract schema definitions
func extractV2Schemas(v2Raw []byte) map[string]any {
    var doc map[string]any
    if err := yaml.Unmarshal(v2Raw, &doc); err != nil {
        return nil
    }
    
    definitions, ok := doc["definitions"].(map[string]any)
    if !ok {
        return nil
    }
    
    return definitions
}

// toSchemaOrRefFromV2 converts a v2 schema definition to our internal SchemaOrRef
func toSchemaOrRefFromV2(v2Schema any, name string) *SchemaOrRef {
    schemaMap, ok := v2Schema.(map[string]any)
    if !ok {
        return nil
    }
    
    schema := &Schema{
        Name: name,
    }
    
    // Extract basic properties
    if typ, ok := schemaMap["type"].(string); ok {
        schema.Type = typ
    }
    
    if desc, ok := schemaMap["description"].(string); ok {
        schema.Description = desc
    }
    
    if format, ok := schemaMap["format"].(string); ok {
        schema.Format = format
    }
    
    if example := schemaMap["example"]; example != nil {
        schema.Example = example
    }
    
    // Handle enum
    if enumVal, ok := schemaMap["enum"].([]any); ok {
        schema.Enum = enumVal
    }
    
    // Handle required fields
    if reqVal, ok := schemaMap["required"].([]any); ok {
        required := make([]string, 0, len(reqVal))
        for _, r := range reqVal {
            if s, ok := r.(string); ok {
                required = append(required, s)
            }
        }
        schema.Required = required
    }
    
    // Handle properties
    if props, ok := schemaMap["properties"].(map[string]any); ok {
        schema.Properties = make(map[string]*SchemaOrRef, len(props))
        for propName, propDef := range props {
            if propSchema := toSchemaOrRefFromV2(propDef, ""); propSchema != nil {
                schema.Properties[propName] = propSchema
            }
        }
    }
    
    // Handle items for arrays
    if items, ok := schemaMap["items"]; ok {
        if itemSchema := toSchemaOrRefFromV2(items, ""); itemSchema != nil {
            schema.Items = itemSchema
        }
    }
    
    // Handle $ref
    if ref, ok := schemaMap["$ref"].(string); ok {
        // Convert Swagger 2.0 definition refs to OpenAPI 3.0 format
        if strings.HasPrefix(ref, "#/definitions/") {
            newRef := strings.Replace(ref, "#/definitions/", "#/components/schemas/", 1)
            return &SchemaOrRef{
                Ref: &SchemaRef{Ref: newRef},
            }
        }
        return &SchemaOrRef{
            Ref: &SchemaRef{Ref: ref},
        }
    }
    
    // Handle allOf, anyOf, oneOf
    if allOf, ok := schemaMap["allOf"].([]any); ok {
        for _, item := range allOf {
            if itemSchema := toSchemaOrRefFromV2(item, ""); itemSchema != nil {
                schema.AllOf = append(schema.AllOf, itemSchema)
            }
        }
    }
    
    if anyOf, ok := schemaMap["anyOf"].([]any); ok {
        for _, item := range anyOf {
            if itemSchema := toSchemaOrRefFromV2(item, ""); itemSchema != nil {
                schema.AnyOf = append(schema.AnyOf, itemSchema)
            }
        }
    }
    
    if oneOf, ok := schemaMap["oneOf"].([]any); ok {
        for _, item := range oneOf {
            if itemSchema := toSchemaOrRefFromV2(item, ""); itemSchema != nil {
                schema.OneOf = append(schema.OneOf, itemSchema)
            }
        }
    }
    
    return &SchemaOrRef{Schema: schema}
}

// toMediaListWithV2Cache enhances media list with cached v2 operations
func toMediaListWithV2Cache(content openapi3.Content, v2Operations map[string]map[string]any, path, method string) []Media {
    // First get the standard conversion
    result := toMediaList(content)
    
    // For performance: only enhance if we actually need to (has requestBody with basic content)
    if len(result) == 0 || result[0].Schema == nil || result[0].Schema.Schema == nil {
        return result
    }
    
    // Quick check: if schema already has proper reference, no need to enhance
    if result[0].Schema.Ref != nil {
        return result
    }
    
    // Only enhance if schema type is basic object without properties
    if result[0].Schema.Schema.Type != "object" || len(result[0].Schema.Schema.Properties) > 0 {
        return result
    }
    
    // Look for the specific operation in cached data
    if pathOps, found := v2Operations[path]; found {
        if methodOp, found := pathOps[strings.ToLower(method)]; found {
            // For request body, look for body parameter
            if strings.ToLower(method) != "get" {
                if bodyParam := findBodyParameter(methodOp); bodyParam != nil {
                    // Update the first media item with the correct schema
                    if len(result) > 0 && bodyParam["schema"] != nil {
                        if schemaRef := bodyParam["schema"].(map[string]any); schemaRef != nil {
                            if ref, ok := schemaRef["$ref"].(string); ok {
                                // Convert the reference
                                refName := strings.Replace(ref, "#/definitions/", "", 1)
                                result[0].Schema = &SchemaOrRef{
                                    Ref: &SchemaRef{
                                        Ref: "#/components/schemas/" + refName,
                                    },
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    
    return result
}


// extractV2Operations parses v2 YAML to extract operation definitions
func extractV2Operations(v2Raw []byte) map[string]map[string]any {
    var doc map[string]any
    if err := yaml.Unmarshal(v2Raw, &doc); err != nil {
        return nil
    }
    
    paths, ok := doc["paths"].(map[string]any)
    if !ok {
        return nil
    }
    
    result := make(map[string]map[string]any)
    for path, pathDef := range paths {
        if pathDefMap, ok := pathDef.(map[string]any); ok {
            operations := make(map[string]any)
            for method, methodDef := range pathDefMap {
                if methodDefMap, ok := methodDef.(map[string]any); ok {
                    operations[strings.ToLower(method)] = methodDefMap
                }
            }
            if len(operations) > 0 {
                result[path] = operations
            }
        }
    }
    
    return result
}

// findBodyParameter finds body parameter in v2 operation definition
func findBodyParameter(operation any) map[string]any {
    opMap, ok := operation.(map[string]any)
    if !ok {
        return nil
    }
    
    params, ok := opMap["parameters"].([]any)
    if !ok {
        return nil
    }
    
    for _, param := range params {
        if paramMap, ok := param.(map[string]any); ok {
            if in, ok := paramMap["in"].(string); ok && in == "body" {
                return paramMap
            }
        }
    }
    
    return nil
}
