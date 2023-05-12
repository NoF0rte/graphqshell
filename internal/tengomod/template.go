package tengomod

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/NoF0rte/graphqshell/pkg/graphql"
)

var funcMap template.FuncMap

var (
	rootSigTemplate  *template.Template
	objSigTemplate   *template.Template
	fieldSigTemplate *template.Template
)

func init() {
	funcMap = template.FuncMap{
		"argSignature": func(objs []*graphql.Object) string {
			if len(objs) == 0 {
				return ""
			}

			var args []string
			for _, arg := range objs {
				args = append(args, fmt.Sprintf("%s: %s", arg.Name, arg.Type))
			}

			return fmt.Sprintf("(%s)", strings.Join(args, ", "))
		},
		"fieldSignature": func(obj *graphql.Object) (string, error) {
			buf := new(bytes.Buffer)
			err := fieldSigTemplate.Execute(buf, obj)
			if err != nil {
				return "", err
			}

			return buf.String(), nil
		},
		"indent": func(v string) string {
			pad := strings.Repeat("\t", 1)
			return pad + strings.Replace(v, "\n", "\n"+pad, -1)
		},
	}

	rootSigTemplateStr := `{{.Name}} {
	{{- range .Items -}}
		{{ fieldSignature . | printf "\n%s" | indent }}
	{{- end }}
}`

	rootSigTemplate = template.Must(template.New("rootSig").Funcs(funcMap).Parse(rootSigTemplateStr))

	fieldSigTemplateStr := `
{{- if ne (len .Description) 0 -}}
	// {{ .Description | println }}
{{- end -}}
{{.Name}}{{.Args | argSignature}}: {{.Type}}`

	fieldSigTemplate = template.Must(template.New("fieldSig").Funcs(funcMap).Parse(fieldSigTemplateStr))

	objSigTemplateStr := `
{{- if ne (len .Description) 0 -}}
	// {{ .Description | println }}
{{- end -}}
{{.Name}}{{.Args | argSignature}} {
	{{- range .Fields -}}
		{{ fieldSignature . | printf "\n%s" | indent }}
	{{- end }}
}`

	objSigTemplate = template.Must(template.New("objSig").Funcs(funcMap).Parse(objSigTemplateStr))
}
