package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sleepyfran/recetor/internal/recipes"
)

func TestToolsEndToEnd(t *testing.T) {
	ctx := context.Background()
	store, err := recipes.Open(ctx, filepath.Join(t.TempDir(), "recipes.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	server := New(store)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect() error = %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "recetor-test", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect() error = %v", err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	gotNames := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		gotNames = append(gotNames, tool.Name)
		if tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Errorf("tool %q is missing a schema", tool.Name)
		}
	}
	sort.Strings(gotNames)
	wantNames := []string{
		"create_recipe",
		"list_recipes",
		"search_recipes_by_ingredients",
		"search_recipes_by_name",
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Fatalf("tool names = %v, want %v", gotNames, wantNames)
		}
	}

	created, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_recipe",
		Arguments: map[string]any{
			"name":        "Pasta",
			"ingredients": []string{"Pasta", "Tomatoes"},
		},
	})
	if err != nil || created.IsError {
		t.Fatalf("create_recipe result = %#v, error = %v", created, err)
	}
	var createOutput createRecipeOutput
	decodeStructured(t, created.StructuredContent, &createOutput)
	if createOutput.Recipe.Name != "Pasta" || createOutput.Recipe.ID == 0 {
		t.Fatalf("create_recipe output = %#v", createOutput)
	}

	found, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_recipes_by_ingredients",
		Arguments: map[string]any{"ingredients": []string{"tomatoes"}},
	})
	if err != nil || found.IsError {
		t.Fatalf("search result = %#v, error = %v", found, err)
	}
	var searchOutput recipesOutput
	decodeStructured(t, found.StructuredContent, &searchOutput)
	if len(searchOutput.Recipes) != 1 || searchOutput.Recipes[0].Name != "Pasta" {
		t.Fatalf("search output = %#v", searchOutput)
	}

	duplicate, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_recipe",
		Arguments: map[string]any{
			"name":        "pasta",
			"ingredients": []string{"Noodles"},
		},
	})
	if err != nil {
		t.Fatalf("duplicate create protocol error = %v", err)
	}
	if !duplicate.IsError || len(duplicate.Content) == 0 {
		t.Fatalf("duplicate create result = %#v, want visible tool error", duplicate)
	}
}

func decodeStructured(t *testing.T, value any, target any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
}
