package openapi

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/gin-gonic/gin"
)

var ginParamRe = regexp.MustCompile(`:(\w+)`)

// Spec builds an OpenAPI 3.0 document incrementally from route registrations.
// Routes registered via its GET/POST/PUT/DELETE methods are both added to the
// gin router AND documented in the spec. Routes registered directly on gin
// groups continue to work but won't appear in the spec.
type Spec struct {
	doc *openapi3.T
}

// New creates a new OpenAPI spec builder.
func New(title, version, description string) *Spec {
	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:       title,
			Version:     version,
			Description: description,
		},
		Paths: openapi3.NewPaths(),
		Components: &openapi3.Components{
			Schemas: openapi3.Schemas{},
			SecuritySchemes: openapi3.SecuritySchemes{
				"bearerAuth": &openapi3.SecuritySchemeRef{
					Value: &openapi3.SecurityScheme{
						Type:         "http",
						Scheme:       "bearer",
						BearerFormat: "JWT",
					},
				},
			},
		},
	}
	return &Spec{doc: doc}
}

// --- Options ---

// Option configures an OpenAPI operation.
type Option func(*opCfg)

type opCfg struct {
	description string
	tags        []string
	body        any
	responses   map[int]any
	pathParams  []paramCfg
	queryParams []paramCfg
	security    bool
	deprecated  bool
}

type paramCfg struct {
	name        string
	description string
	required    bool
}

func Tags(tags ...string) Option {
	return func(c *opCfg) { c.tags = append(c.tags, tags...) }
}

func Desc(d string) Option {
	return func(c *opCfg) { c.description = d }
}

func Body(v any) Option {
	return func(c *opCfg) { c.body = v }
}

func Response(status int, v any) Option {
	return func(c *opCfg) { c.responses[status] = v }
}

func PathParam(name, description string) Option {
	return func(c *opCfg) {
		c.pathParams = append(c.pathParams, paramCfg{name: name, description: description, required: true})
	}
}

func QueryParam(name, description string, required bool) Option {
	return func(c *opCfg) {
		c.queryParams = append(c.queryParams, paramCfg{name: name, description: description, required: required})
	}
}

func BearerAuth() Option {
	return func(c *opCfg) { c.security = true }
}

func Deprecated() Option {
	return func(c *opCfg) { c.deprecated = true }
}

// --- Route registration ---

func (s *Spec) GET(g *gin.RouterGroup, path, summary string, h gin.HandlerFunc, opts ...Option) {
	g.GET(path, h)
	s.add("GET", g.BasePath()+path, summary, opts)
}

func (s *Spec) POST(g *gin.RouterGroup, path, summary string, h gin.HandlerFunc, opts ...Option) {
	g.POST(path, h)
	s.add("POST", g.BasePath()+path, summary, opts)
}

func (s *Spec) PUT(g *gin.RouterGroup, path, summary string, h gin.HandlerFunc, opts ...Option) {
	g.PUT(path, h)
	s.add("PUT", g.BasePath()+path, summary, opts)
}

func (s *Spec) PATCH(g *gin.RouterGroup, path, summary string, h gin.HandlerFunc, opts ...Option) {
	g.PATCH(path, h)
	s.add("PATCH", g.BasePath()+path, summary, opts)
}

func (s *Spec) DELETE(g *gin.RouterGroup, path, summary string, h gin.HandlerFunc, opts ...Option) {
	g.DELETE(path, h)
	s.add("DELETE", g.BasePath()+path, summary, opts)
}

func (s *Spec) add(method, fullPath, summary string, opts []Option) {
	cfg := &opCfg{responses: make(map[int]any)}
	for _, o := range opts {
		o(cfg)
	}

	op := &openapi3.Operation{
		Summary:     summary,
		Description: cfg.description,
		Tags:        cfg.tags,
		Deprecated:  cfg.deprecated,
		Responses:   openapi3.NewResponses(),
	}

	// Auto-detect path params from gin :param patterns
	detected := make(map[string]bool)
	for _, m := range ginParamRe.FindAllStringSubmatch(fullPath, -1) {
		detected[m[1]] = true
	}

	// Add explicitly documented path params
	documented := make(map[string]bool)
	for _, p := range cfg.pathParams {
		documented[p.name] = true
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        p.name,
				In:          "path",
				Required:    true,
				Description: p.description,
				Schema:      openapi3.NewStringSchema().NewRef(),
			},
		})
	}

	// Auto-add path params that weren't explicitly documented
	for name := range detected {
		if !documented[name] {
			op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
				Value: &openapi3.Parameter{
					Name:     name,
					In:       "path",
					Required: true,
					Schema:   openapi3.NewStringSchema().NewRef(),
				},
			})
		}
	}

	// Query params
	for _, p := range cfg.queryParams {
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:        p.name,
				In:          "query",
				Required:    p.required,
				Description: p.description,
				Schema:      openapi3.NewStringSchema().NewRef(),
			},
		})
	}

	// Request body
	if cfg.body != nil {
		schemaRef, err := openapi3gen.NewSchemaRefForValue(cfg.body, s.doc.Components.Schemas)
		if err == nil {
			op.RequestBody = &openapi3.RequestBodyRef{
				Value: &openapi3.RequestBody{
					Required: true,
					Content:  openapi3.NewContentWithJSONSchemaRef(schemaRef),
				},
			}
		}
	}

	// Responses
	for status, v := range cfg.responses {
		desc := http.StatusText(status)
		if v == nil {
			op.Responses.Set(strconv.Itoa(status), &openapi3.ResponseRef{
				Value: &openapi3.Response{Description: &desc},
			})
		} else {
			schemaRef, err := openapi3gen.NewSchemaRefForValue(v, s.doc.Components.Schemas)
			if err == nil {
				op.Responses.Set(strconv.Itoa(status), &openapi3.ResponseRef{
					Value: &openapi3.Response{
						Description: &desc,
						Content:     openapi3.NewContentWithJSONSchemaRef(schemaRef),
					},
				})
			}
		}
	}

	// Security
	if cfg.security {
		op.Security = &openapi3.SecurityRequirements{
			openapi3.SecurityRequirement{"bearerAuth": {}},
		}
	}

	// Convert gin :param to OpenAPI {param} and register on path item
	openAPIPath := ginParamRe.ReplaceAllString(fullPath, `{$1}`)

	pathItem := s.doc.Paths.Find(openAPIPath)
	if pathItem == nil {
		pathItem = &openapi3.PathItem{}
		s.doc.Paths.Set(openAPIPath, pathItem)
	}

	switch method {
	case "GET":
		pathItem.Get = op
	case "POST":
		pathItem.Post = op
	case "PUT":
		pathItem.Put = op
	case "DELETE":
		pathItem.Delete = op
	case "PATCH":
		pathItem.Patch = op
	case "HEAD":
		pathItem.Head = op
	}
}

// JSON returns a gin handler that serves the OpenAPI spec as JSON.
func (s *Spec) JSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.doc)
	}
}
