package spec

import (
    "strings"

    "gopkg.in/yaml.v3"
)

// preprocessV2ForCompatibility rewrites non‑compliant Swagger v2 operations so kin-openapi
// can convert them to v3. Specifically:
// - If an operation contains multiple body parameters, merge them into a single body
//   parameter whose schema is an object with properties per original parameter.
// - If an operation mixes body and formData parameters, convert all body parameters to
//   formData equivalents and ensure the operation consumes multipart/form-data.
//
// It returns possibly-modified YAML bytes, a flag indicating whether modifications were made,
// and any error encountered during parsing/serialization. On error, the original bytes are
// returned with modified=false.
func preprocessV2ForCompatibility(data []byte) ([]byte, bool, error) {
    var doc map[string]any
    if err := yaml.Unmarshal(data, &doc); err != nil {
        return data, false, err
    }
    paths, ok := doc["paths"].(map[string]any)
    if !ok || len(paths) == 0 {
        return data, false, nil
    }
    modified := false

    // Iterate each path + method
    for _, pim := range paths {
        pi, ok := pim.(map[string]any)
        if !ok { continue }
        for method, opm := range pi {
            ml := strings.ToLower(method)
            switch ml {
            case "get", "post", "put", "delete", "patch", "options", "head":
            default:
                continue
            }
            op, ok := opm.(map[string]any)
            if !ok { continue }
            params, ok := op["parameters"].([]any)
            if !ok || len(params) == 0 { continue }

            bodyCount := 0
            hasFormData := false
            for _, p := range params {
                pm, _ := p.(map[string]any)
                if pm == nil { continue }
                if strings.EqualFold(asString(pm["in"]), "body") {
                    bodyCount++
                } else if strings.EqualFold(asString(pm["in"]), "formData") {
                    hasFormData = true
                }
            }

            if bodyCount == 0 { continue }

            if hasFormData {
                // Convert all body parameters to formData to avoid mixing.
                newParams := make([]any, 0, len(params))
                for _, p := range params {
                    pm, _ := p.(map[string]any)
                    if pm == nil { continue }
                    if strings.EqualFold(asString(pm["in"]), "body") {
                        newParams = append(newParams, formDataFromBodyParam(pm))
                        modified = true
                        continue
                    }
                    newParams = append(newParams, pm)
                }
                op["parameters"] = newParams
                // Ensure consumes contains multipart/form-data
                var consumes []any
                if c, ok := op["consumes"].([]any); ok {
                    consumes = c
                }
                if !containsString(consumes, "multipart/form-data") {
                    op["consumes"] = append(consumes, "multipart/form-data")
                }
                continue
            }

            if bodyCount > 1 {
                // Merge multiple body params into a single body schema object.
                props := map[string]any{}
                required := make([]any, 0)
                newParams := make([]any, 0, len(params))
                for _, p := range params {
                    pm, _ := p.(map[string]any)
                    if pm == nil { continue }
                    if strings.EqualFold(asString(pm["in"]), "body") {
                        name := asString(pm["name"])
                        if name == "" { name = "field" }
                        schema := extractSchemaFromParam(pm)
                        if schema == nil { schema = map[string]any{"type": "string"} }
                        props[name] = schema
                        if rb, _ := pm["required"].(bool); rb {
                            required = append(required, name)
                        }
                        modified = true
                        continue
                    }
                    newParams = append(newParams, p)
                }
                bodySchema := map[string]any{"type": "object", "properties": props}
                if len(required) > 0 { bodySchema["required"] = required }
                merged := map[string]any{
                    "in":     "body",
                    "name":   "body",
                    "schema": bodySchema,
                }
                // prepend merged body parameter
                op["parameters"] = append([]any{merged}, newParams...)
                continue
            }
        }
    }

    if !modified {
        return data, false, nil
    }
    out, err := yaml.Marshal(doc)
    if err != nil {
        return data, false, err
    }
    return out, true, nil
}

func asString(v any) string {
    if s, ok := v.(string); ok { return s }
    return ""
}

func containsString(list []any, want string) bool {
    for _, v := range list {
        if s, ok := v.(string); ok && s == want { return true }
    }
    return false
}

func extractSchemaFromParam(pm map[string]any) map[string]any {
    if sch, ok := pm["schema"].(map[string]any); ok {
        return sch
    }
    // Synthesize schema from param type/items/format when present
    t, _ := pm["type"].(string)
    if t == "" { return nil }
    m := map[string]any{"type": t}
    if it, ok := pm["items"].(map[string]any); ok {
        m["items"] = it
    }
    if f, ok := pm["format"].(string); ok && f != "" {
        m["format"] = f
    }
    return m
}

func formDataFromBodyParam(pm map[string]any) map[string]any {
    name := asString(pm["name"])
    if name == "" { name = "field" }
    out := map[string]any{
        "in":   "formData",
        "name": name,
    }
    if desc, ok := pm["description"].(string); ok && desc != "" {
        out["description"] = desc
    }
    if req, ok := pm["required"].(bool); ok {
        out["required"] = req
    }
    // Derive a formData-compatible type; fallback to string.
    var typ string
    var format string
    var items any
    if sch, ok := pm["schema"].(map[string]any); ok {
        if t, ok := sch["type"].(string); ok { typ = t }
        if it, ok := sch["items"].(map[string]any); ok { items = it }
        if f, ok := sch["format"].(string); ok { format = f }
        if typ == "" && sch["$ref"] != nil {
            // Cannot represent a referenced object in formData; degrade to string.
            typ = "string"
        }
    }
    if typ == "" {
        if t, ok := pm["type"].(string); ok { typ = t }
        if it, ok := pm["items"].(map[string]any); ok { items = it }
        if f, ok := pm["format"].(string); ok { format = f }
    }
    if typ == "" { typ = "string" }
    out["type"] = typ
    if items != nil { out["items"] = items }
    if format != "" { out["format"] = format }
    return out
}

