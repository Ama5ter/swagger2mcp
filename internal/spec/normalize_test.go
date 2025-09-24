package spec

import (
    "context"
    "strings"
    "testing"

    "github.com/getkin/kin-openapi/openapi3"
)

const sampleSpec = `openapi: 3.0.0
info:
  title: Sample API
  version: "1.0.0"
  description: Demo
paths:
  /pets:
    parameters:
      - in: query
        name: limit
        required: false
        schema:
          type: integer
    get:
      summary: List pets
      description: Returns all pets
      tags: [read, animal]
      parameters:
        - in: query
          name: limit
          required: true
          schema:
            type: integer
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Pet'
    post:
      summary: Create pet
      tags: [write, animal]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pet'
            example:
              id: 1
              name: Fluffy
      responses:
        "201":
          description: created
  /admin:
    get:
      summary: Admin only
      tags: [admin]
      responses:
        "200": { description: ok }
components:
  schemas:
    Pet:
      type: object
      required: [id, name]
      properties:
        id:
          type: integer
          format: int64
        name:
          type: string
`

func loadDoc(t *testing.T, spec string) *openapi3.T {
    t.Helper()
    loader := openapi3.NewLoader()
    doc, err := loader.LoadFromData([]byte(strings.TrimSpace(spec)))
    if err != nil {
        t.Fatalf("load: %v", err)
    }
    if err := doc.Validate(context.Background()); err != nil {
        t.Fatalf("validate: %v", err)
    }
    return doc
}

func TestBuildServiceModel_Basic(t *testing.T) {
    t.Parallel()
    doc := loadDoc(t, sampleSpec)

    sm, err := BuildServiceModel(context.Background(), doc, nil)
    if err != nil {
        t.Fatalf("build: %v", err)
    }

    if sm.Title != "Sample API" {
        t.Errorf("title: got %q", sm.Title)
    }
    if len(sm.Endpoints) != 3 { // GET /pets, POST /pets, GET /admin
        t.Fatalf("endpoints: got %d", len(sm.Endpoints))
    }

    // Check schema mapping
    pet, ok := sm.Schemas["Pet"]
    if !ok {
        t.Fatalf("schemas: missing Pet")
    }
    if pet.Type != "object" {
        t.Errorf("pet.type: got %q", pet.Type)
    }
    if _, ok := pet.Properties["id"]; !ok {
        t.Errorf("pet.properties: missing id")
    }
    if _, ok := pet.Properties["name"]; !ok {
        t.Errorf("pet.properties: missing name")
    }

    // Ensure request body + response content captured for POST /pets
    var postFound bool
    for _, ep := range sm.Endpoints {
        if ep.Method == POST && ep.Path == "/pets" {
            postFound = true
            if ep.RequestBody == nil || !ep.RequestBody.Required {
                t.Fatalf("post /pets: expected required request body")
            }
            if len(ep.RequestBody.Content) == 0 || ep.RequestBody.Content[0].Mime != "application/json" {
                t.Fatalf("post /pets: expected JSON content")
            }
            if ep.RequestBody.Content[0].Example == nil {
                t.Fatalf("post /pets: expected example value")
            }
        }
        if ep.Method == GET && ep.Path == "/pets" {
            // Parameter merging: operation-level 'limit' overrides required=true
            if len(ep.Parameters) == 0 {
                t.Fatalf("get /pets: expected parameters")
            }
            found := false
            for _, p := range ep.Parameters {
                if p.In == "query" && p.Name == "limit" {
                    found = true
                    if !p.Required {
                        t.Fatalf("get /pets: expected limit to be required after override")
                    }
                }
            }
            if !found {
                t.Fatalf("get /pets: limit parameter not found")
            }
        }
    }
    if !postFound {
        t.Fatalf("post /pets: not found")
    }
}

func TestBuildServiceModel_TagFiltering(t *testing.T) {
    t.Parallel()
    doc := loadDoc(t, sampleSpec)

    // Include only 'read' tagged endpoints (GET /pets)
    sm, err := BuildServiceModel(context.Background(), doc, nil, WithIncludeTags([]string{"read"}))
    if err != nil {
        t.Fatalf("build: %v", err)
    }
    if len(sm.Endpoints) != 1 {
        t.Fatalf("include tags: expected 1 endpoint, got %d", len(sm.Endpoints))
    }
    if sm.Endpoints[0].Method != GET || sm.Endpoints[0].Path != "/pets" {
        t.Fatalf("include tags: wrong endpoint %s %s", sm.Endpoints[0].Method, sm.Endpoints[0].Path)
    }
    // Tags collection should only include tags from included endpoints
    if len(sm.Tags) == 0 || sm.Tags[0] != "animal" {
        t.Fatalf("tags: expected to contain 'animal', got %v", sm.Tags)
    }

    // Exclude 'admin' should remove /admin
    sm2, err := BuildServiceModel(context.Background(), doc, nil, WithExcludeTags([]string{"admin"}))
    if err != nil {
        t.Fatalf("build2: %v", err)
    }
    for _, ep := range sm2.Endpoints {
        if ep.Path == "/admin" {
            t.Fatalf("exclude tags: /admin should be filtered out")
        }
    }
}

func TestBuildServiceModel_MethodAndPathFilters(t *testing.T) {
    t.Parallel()
    doc := loadDoc(t, sampleSpec)

    sm, err := BuildServiceModel(context.Background(), doc, nil, WithMethods([]HttpMethod{POST}), WithPathPatterns([]string{"^/pets$"}))
    if err != nil {
        t.Fatalf("build: %v", err)
    }
    if len(sm.Endpoints) != 1 {
        t.Fatalf("filters: expected 1 endpoint, got %d", len(sm.Endpoints))
    }
    if sm.Endpoints[0].Method != POST || sm.Endpoints[0].Path != "/pets" {
        t.Fatalf("filters: wrong endpoint %s %s", sm.Endpoints[0].Method, sm.Endpoints[0].Path)
    }
}

