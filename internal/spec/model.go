package spec

// Internal Model (IM) definitions used by generators and emitters.

type HttpMethod string

const (
    GET     HttpMethod = "get"
    POST    HttpMethod = "post"
    PUT     HttpMethod = "put"
    DELETE  HttpMethod = "delete"
    PATCH   HttpMethod = "patch"
    HEAD    HttpMethod = "head"
    OPTIONS HttpMethod = "options"
    TRACE   HttpMethod = "trace"
)

type ServiceModel struct {
    Title       string
    Version     string
    Description string
    Servers     []Server
    Tags        []string
    Endpoints   []EndpointModel
    Schemas     map[string]Schema // by name/ref
}

type Server struct {
    URL         string
    Description string
}

type EndpointModel struct {
    ID          string // method+path
    Method      HttpMethod
    Path        string
    Summary     string
    Description string
    Tags        []string
    Parameters  []ParameterModel
    RequestBody *RequestBodyModel
    Responses   []ResponseModel
}

type ParameterModel struct {
    Name     string
    In       string // path|query|header|cookie
    Required bool
    Schema   *SchemaOrRef
}

type RequestBodyModel struct {
    Content  []Media
    Required bool
}

type ResponseModel struct {
    Status      string // 200, 4xx, default
    Description string
    Content     []Media
}

type Media struct {
    Mime   string
    Schema *SchemaOrRef
    // Example holds a single example value if available. It may be nil.
    Example any
}

type Schema struct {
    Name        string
    Type        string
    Properties  map[string]*SchemaOrRef
    Required    []string
    Items       *SchemaOrRef
    AllOf       []*SchemaOrRef
    AnyOf       []*SchemaOrRef
    OneOf       []*SchemaOrRef
    Description string
    Enum        []any
    Format      string
    Example     any
}

type SchemaRef struct{ Ref string }

type SchemaOrRef struct {
    Schema *Schema
    Ref    *SchemaRef
}

