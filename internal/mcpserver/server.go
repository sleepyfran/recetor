package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sleepyfran/recetor/internal/recipes"
)

const Version = "0.3.0"

type recipeStore interface {
	Create(context.Context, string, []string) (recipes.Recipe, error)
	Edit(context.Context, int64, string, []string) (recipes.Recipe, error)
	Remove(context.Context, int64) (recipes.Recipe, error)
	List(context.Context) ([]recipes.Recipe, error)
	SearchByName(context.Context, string) ([]recipes.Recipe, error)
	SearchByIngredientText(context.Context, string) ([]recipes.Recipe, error)
	SearchByIngredients(context.Context, []string) ([]recipes.Recipe, error)
}

type createRecipeInput struct {
	Name        string   `json:"name" jsonschema:"Name of the recipe"`
	Ingredients []string `json:"ingredients" jsonschema:"Ordered list of ingredient strings"`
}

type createRecipeOutput struct {
	Recipe recipes.Recipe `json:"recipe"`
}

type editRecipeInput struct {
	ID          int64    `json:"id" jsonschema:"Stable numeric identifier of the recipe to edit"`
	Name        string   `json:"name" jsonschema:"Replacement name of the recipe"`
	Ingredients []string `json:"ingredients" jsonschema:"Replacement ordered list of ingredient strings"`
}

type removeRecipeInput struct {
	ID int64 `json:"id" jsonschema:"Stable numeric identifier of the recipe to remove"`
}

type listRecipesInput struct{}

type recipesOutput struct {
	Recipes []recipes.Recipe `json:"recipes"`
}

type searchByNameInput struct {
	Query string `json:"query" jsonschema:"Case-insensitive substring to find in recipe names"`
}

type searchByIngredientsInput struct {
	Ingredients []string `json:"ingredients" jsonschema:"Ingredients every returned recipe must contain"`
}

type searchByIngredientTextInput struct {
	Query string `json:"query" jsonschema:"Case-insensitive substring to find in ingredient strings"`
}

func New(store recipeStore) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "recetor", Version: Version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_recipe",
		Description: "Create a recipe from a unique name and an ordered list of ingredient strings.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input createRecipeInput) (*mcp.CallToolResult, createRecipeOutput, error) {
		recipe, err := store.Create(ctx, input.Name, input.Ingredients)
		return nil, createRecipeOutput{Recipe: recipe}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_recipe",
		Description: "Replace a recipe's name and ordered ingredients while preserving its stable ID.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input editRecipeInput) (*mcp.CallToolResult, createRecipeOutput, error) {
		recipe, err := store.Edit(ctx, input.ID, input.Name, input.Ingredients)
		return nil, createRecipeOutput{Recipe: recipe}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_recipe",
		Description: "Permanently remove a recipe by its stable numeric ID and return the removed recipe.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input removeRecipeInput) (*mcp.CallToolResult, createRecipeOutput, error) {
		recipe, err := store.Remove(ctx, input.ID)
		return nil, createRecipeOutput{Recipe: recipe}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_recipes",
		Description: "List all available recipes in deterministic name order.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listRecipesInput) (*mcp.CallToolResult, recipesOutput, error) {
		found, err := store.List(ctx)
		return nil, recipesOutput{Recipes: found}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_recipes_by_name",
		Description: "Find recipes whose names contain a case-insensitive query string.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input searchByNameInput) (*mcp.CallToolResult, recipesOutput, error) {
		found, err := store.SearchByName(ctx, input.Query)
		return nil, recipesOutput{Recipes: found}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_recipes_by_ingredient_text",
		Description: "Find recipes with at least one ingredient containing a case-insensitive query string.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input searchByIngredientTextInput) (*mcp.CallToolResult, recipesOutput, error) {
		found, err := store.SearchByIngredientText(ctx, input.Query)
		return nil, recipesOutput{Recipes: found}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_recipes_by_ingredients",
		Description: "Find recipes containing every supplied ingredient, matched as case-insensitive exact strings.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input searchByIngredientsInput) (*mcp.CallToolResult, recipesOutput, error) {
		found, err := store.SearchByIngredients(ctx, input.Ingredients)
		return nil, recipesOutput{Recipes: found}, err
	})

	return server
}
