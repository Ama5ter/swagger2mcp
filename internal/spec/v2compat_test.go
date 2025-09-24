package spec

import (
    "strings"
    "testing"
)

func TestV2Compat_MultipleBodyMerged(t *testing.T) {
    t.Parallel()
    // An operation with two body params (invalid v2) should be merged into a single body schema.
    in := []byte(`swagger: "2.0"
info: { title: t, version: "1.0.0" }
paths:
  /x:
    post:
      parameters:
      - in: body
        name: a
        required: true
        schema: { type: string }
      - in: body
        name: b
        schema: { type: integer }
      responses: { '200': { description: ok } }
`)
    out, changed, err := preprocessV2ForCompatibility(in)
    if err != nil { t.Fatalf("preprocess: %v", err) }
    if !changed { t.Fatalf("expected changes") }
    s := string(out)
    if !strings.Contains(s, "in: body") || !strings.Contains(s, "name: body") {
        t.Fatalf("expected merged single body parameter, got:\n%s", s)
    }
}

func TestV2Compat_BodyAndFormData_ToFormData(t *testing.T) {
    t.Parallel()
    // Mixing body + formData (file) should convert body to formData and add consumes multipart.
    in := []byte(`swagger: "2.0"
info: { title: t, version: "1.0.0" }
paths:
  /upload:
    post:
      parameters:
      - in: body
        name: desc
        schema: { type: string }
      - in: formData
        name: file
        type: file
        required: true
      responses: { '200': { description: ok } }
`)
    out, changed, err := preprocessV2ForCompatibility(in)
    if err != nil { t.Fatalf("preprocess: %v", err) }
    if !changed { t.Fatalf("expected changes") }
    s := string(out)
    if strings.Contains(s, "\n      - in: body\n") {
        t.Fatalf("expected no body params after conversion to formData, got:\n%s", s)
    }
    if !strings.Contains(s, "multipart/form-data") {
        t.Fatalf("expected consumes multipart/form-data, got:\n%s", s)
    }
}
