package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	. "github.com/jnsoft/jnq/src/testhelper"
)

func TestMemPQueue(t *testing.T) {

	t.Run("basic operations", func(t *testing.T) {
		dir, _ := os.Getwd()
		parentDir := dir + "/../../"
		os.Chdir(parentDir)

		main() // generate file
		swaggerFile := "./src/server/swagger.json"

		swaggerBytes, err := os.ReadFile(swaggerFile)
		AssertNoError(t, err)

		fmt.Println(string(swaggerBytes))

		var swagger map[string]interface{}
		err = json.Unmarshal(swaggerBytes, &swagger)

		//swaggerJSON, err := json.MarshalIndent(swagger, "", "  ")
		// fmt.Println(string(swaggerJSON))

		if err != nil {
			t.Fatalf("Failed to parse Swagger JSON: %v", err)
		}

		if swagger["openapi"] != "3.0.4" {
			t.Errorf("Expected openapi version 3.0.4, got %v", swagger["openapi"])
		}
	})
}

/*

	info, ok := swagger["info"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing or invalid 'info' section in Swagger")
	}
	if info["title"] != "Priority Queue API" {
		t.Errorf("Expected title 'Priority Queue API', got %v", info["title"])
	}

	paths, ok := swagger["paths"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing or invalid 'paths' section in Swagger")
	}
	if _, exists := paths["/enqueue"]; !exists {
		t.Errorf("Expected '/enqueue' path to be defined in Swagger")
	}

	// Check for specific parameters in the '/enqueue' path
	enqueuePath, ok := paths["/enqueue"].(map[string]interface{})
	if !ok {
		t.Fatalf("Invalid '/enqueue' path structure")
	}
	postMethod, ok := enqueuePath["post"].(map[string]interface{})
	if !ok {
		t.Fatalf("Missing 'post' method for '/enqueue' path")
	}
	parameters, ok := postMethod["parameters"].([]interface{})
	if !ok {
		t.Fatalf("Missing or invalid 'parameters' for '/enqueue' POST method")
	}

	// Validate specific query parameters
	foundPrio, foundChannel, foundNotBefore := false, false, false
	for _, param := range parameters {
		paramMap, ok := param.(map[string]interface{})
		if !ok {
			continue
		}
		switch paramMap["name"] {
		case "prio":
			foundPrio = true
		case "channel":
			foundChannel = true
		case "notbefore":
			foundNotBefore = true
		}
	}
	if !foundPrio || !foundChannel || !foundNotBefore {
		t.Errorf("Expected query parameters 'prio', 'channel', and 'notbefore' in '/enqueue' POST method")
	}
}

*/
