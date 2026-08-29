package recipes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

var (
	ErrInvalidRecipe  = errors.New("invalid recipe")
	ErrRecipeExists   = errors.New("recipe name already exists")
	ErrRecipeNotFound = errors.New("recipe not found")
)

type Recipe struct {
	ID          int64    `json:"id" jsonschema:"Stable numeric identifier for the recipe"`
	Name        string   `json:"name" jsonschema:"Recipe name"`
	Ingredients []string `json:"ingredients" jsonschema:"Ordered ingredient strings"`
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path must not be empty")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// SQLite pragmas such as foreign_keys are connection-local. A single
	// connection also gives this small local server predictable write behavior.
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initialize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}

	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, schemaVersion)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS recipes (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			normalized_name TEXT NOT NULL UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS ingredients (
			recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
			position INTEGER NOT NULL,
			value TEXT NOT NULL,
			normalized_value TEXT NOT NULL,
			PRIMARY KEY (recipe_id, position),
			UNIQUE (recipe_id, normalized_value)
		)`,
		`CREATE INDEX IF NOT EXISTS ingredients_normalized_value_idx
			ON ingredients(normalized_value)`,
		fmt.Sprintf("PRAGMA user_version = %d", schemaVersion),
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration: %w", err)
	}
	return nil
}

func (s *Store) Create(ctx context.Context, name string, ingredients []string) (Recipe, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Recipe{}, fmt.Errorf("%w: name must not be blank", ErrInvalidRecipe)
	}
	cleanIngredients, normalizedIngredients, err := normalizeIngredients(ingredients, false)
	if err != nil {
		return Recipe{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Recipe{}, fmt.Errorf("begin create recipe: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`INSERT INTO recipes(name, normalized_name) VALUES (?, ?)
		 ON CONFLICT(normalized_name) DO NOTHING`, name, normalize(name))
	if err != nil {
		return Recipe{}, fmt.Errorf("insert recipe: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return Recipe{}, fmt.Errorf("check inserted recipe: %w", err)
	}
	if inserted == 0 {
		return Recipe{}, fmt.Errorf("%w: %q", ErrRecipeExists, name)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Recipe{}, fmt.Errorf("read recipe id: %w", err)
	}

	for i := range cleanIngredients {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ingredients(recipe_id, position, value, normalized_value)
			 VALUES (?, ?, ?, ?)`, id, i, cleanIngredients[i], normalizedIngredients[i]); err != nil {
			return Recipe{}, fmt.Errorf("insert ingredient: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Recipe{}, fmt.Errorf("commit recipe: %w", err)
	}

	return Recipe{ID: id, Name: name, Ingredients: cleanIngredients}, nil
}

// Edit replaces a recipe's name and ordered ingredients while preserving its ID.
func (s *Store) Edit(ctx context.Context, id int64, name string, ingredients []string) (Recipe, error) {
	if id <= 0 {
		return Recipe{}, fmt.Errorf("%w: recipe ID must be positive", ErrInvalidRecipe)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Recipe{}, fmt.Errorf("%w: name must not be blank", ErrInvalidRecipe)
	}
	cleanIngredients, normalizedIngredients, err := normalizeIngredients(ingredients, false)
	if err != nil {
		return Recipe{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Recipe{}, fmt.Errorf("begin edit recipe: %w", err)
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM recipes WHERE id = ?`, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, fmt.Errorf("%w: ID %d", ErrRecipeNotFound, id)
	} else if err != nil {
		return Recipe{}, fmt.Errorf("find recipe to edit: %w", err)
	}

	var conflictingID int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM recipes WHERE normalized_name = ? AND id <> ?`, normalize(name), id).Scan(&conflictingID)
	if err == nil {
		return Recipe{}, fmt.Errorf("%w: %q", ErrRecipeExists, name)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, fmt.Errorf("check recipe name: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE recipes SET name = ?, normalized_name = ? WHERE id = ?`, name, normalize(name), id); err != nil {
		return Recipe{}, fmt.Errorf("update recipe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ingredients WHERE recipe_id = ?`, id); err != nil {
		return Recipe{}, fmt.Errorf("replace ingredients: %w", err)
	}
	for i := range cleanIngredients {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ingredients(recipe_id, position, value, normalized_value)
			 VALUES (?, ?, ?, ?)`, id, i, cleanIngredients[i], normalizedIngredients[i]); err != nil {
			return Recipe{}, fmt.Errorf("insert edited ingredient: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Recipe{}, fmt.Errorf("commit edited recipe: %w", err)
	}

	return Recipe{ID: id, Name: name, Ingredients: cleanIngredients}, nil
}

// Remove deletes a recipe and returns its last stored representation.
func (s *Store) Remove(ctx context.Context, id int64) (Recipe, error) {
	if id <= 0 {
		return Recipe{}, fmt.Errorf("%w: recipe ID must be positive", ErrInvalidRecipe)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Recipe{}, fmt.Errorf("begin remove recipe: %w", err)
	}
	defer tx.Rollback()

	recipe := Recipe{ID: id}
	if err := tx.QueryRowContext(ctx, `SELECT name FROM recipes WHERE id = ?`, id).Scan(&recipe.Name); errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, fmt.Errorf("%w: ID %d", ErrRecipeNotFound, id)
	} else if err != nil {
		return Recipe{}, fmt.Errorf("find recipe to remove: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT value FROM ingredients WHERE recipe_id = ? ORDER BY position`, id)
	if err != nil {
		return Recipe{}, fmt.Errorf("query removed recipe ingredients: %w", err)
	}
	recipe.Ingredients = make([]string, 0)
	for rows.Next() {
		var ingredient string
		if err := rows.Scan(&ingredient); err != nil {
			rows.Close()
			return Recipe{}, fmt.Errorf("scan removed recipe ingredient: %w", err)
		}
		recipe.Ingredients = append(recipe.Ingredients, ingredient)
	}
	if err := rows.Close(); err != nil {
		return Recipe{}, fmt.Errorf("close removed recipe ingredients: %w", err)
	}
	if err := rows.Err(); err != nil {
		return Recipe{}, fmt.Errorf("iterate removed recipe ingredients: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM recipes WHERE id = ?`, id); err != nil {
		return Recipe{}, fmt.Errorf("remove recipe: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Recipe{}, fmt.Errorf("commit removed recipe: %w", err)
	}
	return recipe, nil
}

func (s *Store) List(ctx context.Context) ([]Recipe, error) {
	return s.queryRecipes(ctx,
		`SELECT id, name FROM recipes ORDER BY normalized_name, id`, nil)
}

func (s *Store) SearchByName(ctx context.Context, query string) ([]Recipe, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: name query must not be blank", ErrInvalidRecipe)
	}
	pattern := "%" + escapeLike(normalize(query)) + "%"
	return s.queryRecipes(ctx,
		`SELECT id, name FROM recipes
		 WHERE normalized_name LIKE ? ESCAPE '\'
		 ORDER BY normalized_name, id`, []any{pattern})
}

func (s *Store) SearchByIngredientText(ctx context.Context, query string) ([]Recipe, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: ingredient query must not be blank", ErrInvalidRecipe)
	}
	pattern := "%" + escapeLike(normalize(query)) + "%"
	return s.queryRecipes(ctx,
		`SELECT DISTINCT r.id, r.name
		 FROM recipes r
		 JOIN ingredients i ON i.recipe_id = r.id
		 WHERE i.normalized_value LIKE ? ESCAPE '\'
		 ORDER BY r.normalized_name, r.id`, []any{pattern})
}

func (s *Store) SearchByIngredients(ctx context.Context, ingredients []string) ([]Recipe, error) {
	_, normalized, err := normalizeIngredients(ingredients, true)
	if err != nil {
		return nil, err
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(normalized)), ",")
	query := `SELECT r.id, r.name
		FROM recipes r
		JOIN ingredients i ON i.recipe_id = r.id
		WHERE i.normalized_value IN (` + placeholders + `)
		GROUP BY r.id, r.name, r.normalized_name
		HAVING COUNT(DISTINCT i.normalized_value) = ?
		ORDER BY r.normalized_name, r.id`
	args := make([]any, 0, len(normalized)+1)
	for _, ingredient := range normalized {
		args = append(args, ingredient)
	}
	args = append(args, len(normalized))
	return s.queryRecipes(ctx, query, args)
}

func (s *Store) queryRecipes(ctx context.Context, query string, args []any) ([]Recipe, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query recipes: %w", err)
	}

	recipes := make([]Recipe, 0)
	for rows.Next() {
		var recipe Recipe
		if err := rows.Scan(&recipe.ID, &recipe.Name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan recipe: %w", err)
		}
		recipes = append(recipes, recipe)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close recipe rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recipes: %w", err)
	}

	for i := range recipes {
		ingredientRows, err := s.db.QueryContext(ctx,
			`SELECT value FROM ingredients WHERE recipe_id = ? ORDER BY position`, recipes[i].ID)
		if err != nil {
			return nil, fmt.Errorf("query ingredients: %w", err)
		}
		recipes[i].Ingredients = make([]string, 0)
		for ingredientRows.Next() {
			var ingredient string
			if err := ingredientRows.Scan(&ingredient); err != nil {
				ingredientRows.Close()
				return nil, fmt.Errorf("scan ingredient: %w", err)
			}
			recipes[i].Ingredients = append(recipes[i].Ingredients, ingredient)
		}
		if err := ingredientRows.Close(); err != nil {
			return nil, fmt.Errorf("close ingredient rows: %w", err)
		}
		if err := ingredientRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate ingredients: %w", err)
		}
	}
	return recipes, nil
}

func normalizeIngredients(ingredients []string, deduplicate bool) ([]string, []string, error) {
	if len(ingredients) == 0 {
		return nil, nil, fmt.Errorf("%w: at least one ingredient is required", ErrInvalidRecipe)
	}
	clean := make([]string, 0, len(ingredients))
	normalized := make([]string, 0, len(ingredients))
	seen := make(map[string]struct{}, len(ingredients))
	for i, ingredient := range ingredients {
		ingredient = strings.TrimSpace(ingredient)
		if ingredient == "" {
			return nil, nil, fmt.Errorf("%w: ingredient %d must not be blank", ErrInvalidRecipe, i+1)
		}
		norm := normalize(ingredient)
		if _, exists := seen[norm]; exists {
			if deduplicate {
				continue
			}
			return nil, nil, fmt.Errorf("%w: duplicate ingredient %q", ErrInvalidRecipe, ingredient)
		}
		seen[norm] = struct{}{}
		clean = append(clean, ingredient)
		normalized = append(normalized, norm)
	}
	return clean, normalized, nil
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
