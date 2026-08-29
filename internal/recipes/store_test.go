package recipes

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recipes.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store, path
}

func TestCreateListAndPersist(t *testing.T) {
	ctx := context.Background()
	store, path := openTestStore(t)

	bananaBread, err := store.Create(ctx, "  Banana Bread  ", []string{" 3 bananas ", "Flour"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if bananaBread.ID == 0 || bananaBread.Name != "Banana Bread" {
		t.Fatalf("Create() = %#v", bananaBread)
	}
	if !reflect.DeepEqual(bananaBread.Ingredients, []string{"3 bananas", "Flour"}) {
		t.Fatalf("ingredients = %#v", bananaBread.Ingredients)
	}
	if _, err := store.Create(ctx, "apple pie", []string{"Apples"}); err != nil {
		t.Fatalf("Create() second recipe error = %v", err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if names := recipeNames(got); !reflect.DeepEqual(names, []string{"apple pie", "Banana Bread"}) {
		t.Fatalf("List() names = %#v", names)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer reopened.Close()
	got, err = reopened.List(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("persisted List() = %#v, %v", got, err)
	}
}

func TestCreateValidationAndUniqueness(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)

	if _, err := store.Create(ctx, "Soup", []string{"Water"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Create(ctx, " soup ", []string{"Stock"}); !errors.Is(err, ErrRecipeExists) {
		t.Fatalf("duplicate Create() error = %v, want ErrRecipeExists", err)
	}

	cases := []struct {
		name        string
		recipeName  string
		ingredients []string
	}{
		{"blank name", " ", []string{"Water"}},
		{"no ingredients", "Toast", nil},
		{"blank ingredient", "Toast", []string{"Bread", " "}},
		{"duplicate ingredient", "Toast", []string{"Bread", " bread "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.Create(ctx, tc.recipeName, tc.ingredients); !errors.Is(err, ErrInvalidRecipe) {
				t.Fatalf("Create() error = %v, want ErrInvalidRecipe", err)
			}
		})
	}

	got, err := store.List(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("List() after rejected creates = %#v, %v", got, err)
	}
}

func TestEditRecipe(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)

	created, err := store.Create(ctx, "Soup", []string{"Water", "Salt"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := store.Create(ctx, "Toast", []string{"Bread"}); err != nil {
		t.Fatalf("Create() second recipe error = %v", err)
	}

	edited, err := store.Edit(ctx, created.ID, "  Vegetable Soup ", []string{" Stock ", "Carrots"})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if edited.ID != created.ID || edited.Name != "Vegetable Soup" ||
		!reflect.DeepEqual(edited.Ingredients, []string{"Stock", "Carrots"}) {
		t.Fatalf("Edit() = %#v", edited)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if names := recipeNames(got); !reflect.DeepEqual(names, []string{"Toast", "Vegetable Soup"}) {
		t.Fatalf("List() names = %#v", names)
	}
	if !reflect.DeepEqual(got[1].Ingredients, []string{"Stock", "Carrots"}) {
		t.Fatalf("stored ingredients = %#v", got[1].Ingredients)
	}

	if _, err := store.Edit(ctx, created.ID, " toast ", []string{"Stock"}); !errors.Is(err, ErrRecipeExists) {
		t.Fatalf("duplicate Edit() error = %v, want ErrRecipeExists", err)
	}
	if _, err := store.Edit(ctx, 999, "Missing", []string{"Stock"}); !errors.Is(err, ErrRecipeNotFound) {
		t.Fatalf("missing Edit() error = %v, want ErrRecipeNotFound", err)
	}
	if _, err := store.Edit(ctx, 0, "Invalid", []string{"Stock"}); !errors.Is(err, ErrInvalidRecipe) {
		t.Fatalf("invalid ID Edit() error = %v, want ErrInvalidRecipe", err)
	}

	got, err = store.List(ctx)
	if err != nil || got[1].Name != "Vegetable Soup" || !reflect.DeepEqual(got[1].Ingredients, []string{"Stock", "Carrots"}) {
		t.Fatalf("List() after rejected edits = %#v, %v", got, err)
	}
}

func TestRemoveRecipe(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)

	created, err := store.Create(ctx, "Soup", []string{"Water", "Salt"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	removed, err := store.Remove(ctx, created.ID)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if !reflect.DeepEqual(removed, created) {
		t.Fatalf("Remove() = %#v, want %#v", removed, created)
	}

	got, err := store.List(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("List() after Remove() = %#v, %v", got, err)
	}
	if _, err := store.Remove(ctx, created.ID); !errors.Is(err, ErrRecipeNotFound) {
		t.Fatalf("second Remove() error = %v, want ErrRecipeNotFound", err)
	}
	if _, err := store.Remove(ctx, -1); !errors.Is(err, ErrInvalidRecipe) {
		t.Fatalf("invalid ID Remove() error = %v, want ErrInvalidRecipe", err)
	}
}

func TestSearchByName(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	for name, ingredients := range map[string][]string{
		"Quick Chili": {"Beans"},
		"Chili Oil":   {"Chilies", "Oil"},
		"100% Cake":   {"Flour"},
		"A_B Toast":   {"Bread"},
	} {
		if _, err := store.Create(ctx, name, ingredients); err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"CHILI", []string{"Chili Oil", "Quick Chili"}},
		{"%", []string{"100% Cake"}},
		{"_", []string{"A_B Toast"}},
		{"missing", []string{}},
	}
	for _, tc := range tests {
		got, err := store.SearchByName(ctx, tc.query)
		if err != nil {
			t.Fatalf("SearchByName(%q) error = %v", tc.query, err)
		}
		if names := recipeNames(got); !reflect.DeepEqual(names, tc.want) {
			t.Errorf("SearchByName(%q) = %#v, want %#v", tc.query, names, tc.want)
		}
	}
	if _, err := store.SearchByName(ctx, " "); !errors.Is(err, ErrInvalidRecipe) {
		t.Fatalf("blank SearchByName() error = %v", err)
	}
}

func TestSearchByIngredientsMatchesAll(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	for name, ingredients := range map[string][]string{
		"Pancakes": {"Flour", "Eggs", "Milk"},
		"Omelette": {"Eggs", "Milk"},
		"Toast":    {"Bread"},
	} {
		if _, err := store.Create(ctx, name, ingredients); err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
	}

	tests := []struct {
		ingredients []string
		want        []string
	}{
		{[]string{" eggs ", "MILK"}, []string{"Omelette", "Pancakes"}},
		{[]string{"Eggs", "eggs", "Flour"}, []string{"Pancakes"}},
		{[]string{"Eggs", "Bread"}, []string{}},
	}
	for _, tc := range tests {
		got, err := store.SearchByIngredients(ctx, tc.ingredients)
		if err != nil {
			t.Fatalf("SearchByIngredients(%v) error = %v", tc.ingredients, err)
		}
		if names := recipeNames(got); !reflect.DeepEqual(names, tc.want) {
			t.Errorf("SearchByIngredients(%v) = %#v, want %#v", tc.ingredients, names, tc.want)
		}
	}
	if _, err := store.SearchByIngredients(ctx, nil); !errors.Is(err, ErrInvalidRecipe) {
		t.Fatalf("empty SearchByIngredients() error = %v", err)
	}
}

func TestSearchByIngredientText(t *testing.T) {
	ctx := context.Background()
	store, _ := openTestStore(t)
	for name, ingredients := range map[string][]string{
		"Cake":       {"2 Eggs", "Egg wash", "Flour"},
		"Omelette":   {"1 egg", "Cheese"},
		"Toast":      {"Bread"},
		"Cocoa":      {"100% cocoa"},
		"Underscore": {"A_B seasoning"},
	} {
		if _, err := store.Create(ctx, name, ingredients); err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"EGG", []string{"Cake", "Omelette"}},
		{"2 egg", []string{"Cake"}},
		{"%", []string{"Cocoa"}},
		{"_", []string{"Underscore"}},
		{"missing", []string{}},
	}
	for _, tc := range tests {
		got, err := store.SearchByIngredientText(ctx, tc.query)
		if err != nil {
			t.Fatalf("SearchByIngredientText(%q) error = %v", tc.query, err)
		}
		if names := recipeNames(got); !reflect.DeepEqual(names, tc.want) {
			t.Errorf("SearchByIngredientText(%q) = %#v, want %#v", tc.query, names, tc.want)
		}
	}
	if _, err := store.SearchByIngredientText(ctx, " "); !errors.Is(err, ErrInvalidRecipe) {
		t.Fatalf("blank SearchByIngredientText() error = %v", err)
	}
}

func recipeNames(recipes []Recipe) []string {
	names := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		names = append(names, recipe.Name)
	}
	return names
}
