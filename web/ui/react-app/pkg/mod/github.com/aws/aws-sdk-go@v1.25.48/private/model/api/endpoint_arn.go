package api

import "text/template"

const endpointARNShapeTmplDef = `
{{- define "endpointARNShapeTmpl" }}
{{ range $_, $name := $.MemberNames -}}
	{{ $elem := index $.MemberRefs $name -}}
	{{ if $elem.EndpointARN -}}
		func (s *{{ $.ShapeName }}) getEndpointARN() (arn.Resource, error) {
			if s.{{ $name }} == nil {
				return nil, fmt.Errorf("member {{ $name }} is nil")
			}
			return parseEndpointARN(*s.{{ $name }})
		}

		func (s *{{ $.ShapeName }}) hasEndpointARN() bool {
			if s.{{ $name }} == nil {
				return false
			}
			return arn.IsARN(*s.{{ $name }})
		}
	{{ end -}}
{{ end }}
{{ end }}
`

var endpointARNShapeTmpl = template.Must(
	template.New("endpointARNShapeTmpl").
		Parse(endpointARNShapeTmplDef),
)
