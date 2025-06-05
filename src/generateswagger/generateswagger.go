package main

import (
	"encoding/csv"
	"encoding/json"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Dynamically resolve paths
	basePath, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to determine working directory: %v", err)
	}

	serverDefinition := filepath.Join(basePath, "src/server/server.go")
	outputFile := filepath.Join(basePath, "src/server/swagger.json")

	swagger := map[string]any{
		"openapi": "3.0.4",
		"info": map[string]any{
			"title":       "Priority Queue API",
			"description": "API for managing a priority queue",
			"version":     "1.0.0",
		},
		"paths": map[string]any{},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKeyAuth": map[string]any{
					"type": "apiKey",
					"name": "X-API-Key",
					"in":   "header",
				},
			},
		},
		"security": []map[string]any{
			{
				"ApiKeyAuth": []any{},
			},
		},
	}

	comments := getComments(serverDefinition)

	for _, comment := range comments {
		path, pathItem := parseComment(comment)
		swagger["paths"].(map[string]any)[path] = pathItem
	}

	swaggerBytes, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	// Write the swagger.json file
	err = os.WriteFile(outputFile, swaggerBytes, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func getComments(path string) []string {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	var comments []string
	for _, comment := range node.Comments {
		text := comment.Text()
		if strings.Contains(text, "@Router") {
			comments = append(comments, text)
		}
	}
	return comments
}

func parseComment(comment string) (string, map[string]any) {
	pathItem := make(map[string]any)
	var path, method string

	lines := strings.SplitSeq(comment, "\n")
	for line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
		if strings.HasPrefix(line, "@Router") {
			path = strings.TrimSpace(strings.TrimPrefix(line, "@Router"))
			pathParts := strings.Fields(path)
			if len(pathParts) > 0 {
				path = pathParts[0]
			}
			pathItem["path"] = path
		} else if strings.HasPrefix(line, "@Method") {
			method = strings.TrimSpace(strings.TrimPrefix(line, "@Method"))
			pathItem["method"] = method
		} else if strings.HasPrefix(line, "@Summary") {
			pathItem["summary"] = strings.TrimSpace(strings.TrimPrefix(line, "@Summary"))
		} else if strings.HasPrefix(line, "@Description") {
			pathItem["description"] = strings.TrimSpace(strings.TrimPrefix(line, "@Description"))
		} else if strings.HasPrefix(line, "@Param") {
			param := parseParam(line)
			if param["in"] == "query" || param["in"] == "path" {
				if pathItem["parameters"] == nil {
					pathItem["parameters"] = []map[string]any{}
				}
				pathItem["parameters"] = append(pathItem["parameters"].([]map[string]any), param)
			} else if param["in"] == "body" {
				if pathItem["requestBody"] == nil {
					pathItem["requestBody"] = map[string]any{
						"required": param["required"],
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":        param["schema"].(map[string]any)["type"],
									"format":      param["schema"].(map[string]any)["format"],
									"description": param["description"],
								},
							},
						},
					}
				} else {
					pathItem["requestBody"].(map[string]any)["required"] = param["required"]
					pathItem["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)["description"] = param["description"]
				}
			}
		} else if strings.HasPrefix(line, "@Accept") {

		} else if strings.HasPrefix(line, "@Success") || strings.HasPrefix(line, "@Failure") {
			response := parseResponse(line)
			if pathItem["responses"] == nil {
				pathItem["responses"] = make(map[string]any)
			}

			for status, resp := range response {
				pathItem["responses"].(map[string]any)[status] = resp
			}
		}
	}

	pathItem["security"] = []map[string]any{
		{
			"ApiKeyAuth": []any{},
		},
	}

	return path, map[string]any{
		strings.ToLower(method): pathItem,
	}

}

func parseParam(line string) map[string]any {
	parameter := make(map[string]any)

	reader := csv.NewReader(strings.NewReader(line))
	reader.Comma = ' ' // delimiter
	reader.TrimLeadingSpace = true
	parts, err := reader.Read()
	if err != nil {
		log.Fatalf("Failed to parse line: %v", err)
	}

	if len(parts) > 1 {
		parameter["name"] = parts[1]
	}
	if len(parts) > 2 {
		parameter["in"] = parts[2]
	}
	if len(parts) > 3 {
		typeStr := parts[3]
		schema := map[string]any{}
		if typeStr == "int" {
			schema["type"] = "integer"
			schema["format"] = "int32"
		} else if typeStr == "float" {
			schema["type"] = "number"
			schema["format"] = "float"
		} else if typeStr == "string" {
			schema["type"] = "string"
		} else if typeStr == "bool" {
			schema["type"] = "boolean"
		} else if typeStr == "timestamp" {
			schema["type"] = "string"
			schema["format"] = "date-time"
		} else if typeStr == "json" {
			schema["type"] = "object"
		} else if typeStr == "array" {
			schema["type"] = "array"
			schema["items"] = map[string]any{
				"type": "string",
			}
		} else if typeStr == "object" {
			schema["type"] = "object"
		} else if typeStr == "file" {
			schema["type"] = "string"
			schema["format"] = "binary"
		} else if typeStr == "binary" {
			schema["type"] = "string"
			schema["format"] = "binary"
		} else {
			schema["schema"] = map[string]any{
				"$ref": "#/definitions/" + typeStr,
			}
		}
		parameter["schema"] = schema
	}
	if len(parts) > 4 {
		parameter["required"] = parts[4] == "true"
	}
	if len(parts) > 5 {
		parameter["description"] = parts[5]
	}

	return parameter
}

func parseResponse(line string) map[string]any {
	response := map[string]any{}

	reader := csv.NewReader(strings.NewReader(line))
	reader.Comma = ' ' // delimiter
	reader.TrimLeadingSpace = true

	parts, err := reader.Read()
	if err != nil {
		log.Fatalf("Failed to parse line: %v", err)
	}

	var status, description, content string

	if len(parts) > 1 {
		status = parts[1]
	}

	if len(parts) > 2 {
		description = parts[2]
	}

	if len(parts) > 3 {
		contentString := parts[3]
		if contentString == "json" {
			content = "application/json"
		} else if contentString == "xml" {
			content = "application/xml"
		} else if contentString == "plain" {
			content = "text/plain"
		} else if contentString == "html" {
			content = "text/html"
		} else if contentString == "text" {
			content = "text/plain"
		} else if contentString == "string" {
			content = "text/plain"
		} else if contentString == "binary" {
			content = "application/octet-stream"
		} else if contentString == "form" {
			content = "application/x-www-form-urlencoded"
		} else if contentString == "form-data" {
			content = "multipart/form-data"
		} else if contentString == "urlencoded" {
			content = "application/x-www-form-urlencoded"
		} else if contentString == "multipart" {
			content = "multipart/form-data"
		} else if contentString == "x-www-form-urlencoded" {
			content = "application/x-www-form-urlencoded"
		} else {
			content = contentString
		}
	} else {
		content = "text/plain"
	}

	response[status] = map[string]any{
		"description": description,
		"content": map[string]any{
			content: map[string]any{
				"schema": map[string]any{
					"type": "string",
				},
			},
		},
	}

	return response
}
