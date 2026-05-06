package parser

import (
	"bytes"
	"text/template"
)

func ParseTemplate(templateString string, data any) (string, error) {
	var buf bytes.Buffer
	tmpl, err := template.New("content").Parse(templateString)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func MustParseTemplate(templateString string, data any) string {
	result, err := ParseTemplate(templateString, data)
	if err != nil {
		panic(err)
	}
	return result
}
