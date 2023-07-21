package tengomod

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/NoF0rte/graphqshell/internal/graphql"
)

var funcMap template.FuncMap

var (
	rootSigTemplate          *template.Template
	objSigTemplate           *template.Template
	fieldSigTemplate         *template.Template
	fieldSigWithDescTemplate *template.Template
)

func execTemplate(tpl *template.Template, context interface{}) (string, error) {
	buf := new(bytes.Buffer)
	err := tpl.Execute(buf, context)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

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
			return execTemplate(fieldSigWithDescTemplate, obj)
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

	fieldSigTemplateStr := `{{.Name}}{{.Args | argSignature}}: {{.Type}}`

	fieldSigTemplate = template.Must(template.New("fieldSig").Funcs(funcMap).Parse(fieldSigTemplateStr))

	fieldSigWithDescTemplateStr := fmt.Sprintf(`
{{- if ne (len .Description) 0 -}}
	// {{ .Description | println }}
{{- end -}}
%s`, fieldSigTemplateStr)

	fieldSigWithDescTemplate = template.Must(template.New("fieldSigWithDesc").Funcs(funcMap).Parse(fieldSigWithDescTemplateStr))

	objSigTemplateStr := `
{{- if ne (len .Description) 0 -}}
	// {{ .Description | println }}
{{- end -}}
{{- if ne (len .Fields) 0 -}}
{{.Name}}{{.Args | argSignature}} {
	{{- range .Fields -}}
		{{ fieldSignature . | printf "\n%s" | indent }}
	{{- end }}
}
{{- else -}}
{{ fieldSignature . }}
{{- end -}}`

	objSigTemplate = template.Must(template.New("objSig").Funcs(funcMap).Parse(objSigTemplateStr))
}
