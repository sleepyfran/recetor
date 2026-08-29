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

func recipeNames(recipes []Recipe) []string {
	names := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		names = append(names, recipe.Name)
	}
	return names
}
