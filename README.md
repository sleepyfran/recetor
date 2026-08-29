# recetor

`recetor` is a local [Model Context Protocol](https://modelcontextprotocol.io/) server that stores recipes in SQLite. It gives LLM agents a small, predictable toolset for creating and finding recipes that can later support meal planning and grocery workflows.

It uses the official Go MCP SDK, communicates over stdio, and uses a CGO-free SQLite driver. It has no dependency on any particular agent or MCP client.

## Build and run

Go 1.25 or newer is required.

```sh
go build -o recetor ./cmd/recetor
./recetor --db /absolute/path/to/recipes.db
```

The database path defaults to `recetor.db` in the process working directory. The database and its schema are created automatically. Use an absolute path in MCP client configuration so the location does not depend on how the client was started.

All normal communication uses stdin and stdout for MCP. Startup and runtime errors are written to stderr.

## Tools

| Tool | Input | Behavior |
| --- | --- | --- |
| `create_recipe` | `name`, `ingredients` | Creates and returns a recipe. Names must be unique case-insensitively. |
| `edit_recipe` | `id`, `name`, `ingredients` | Replaces a recipe's name and ingredients while preserving its ID. |
| `remove_recipe` | `id` | Permanently removes and returns a recipe. |
| `list_recipes` | none | Returns every recipe in case-insensitive name order. |
| `search_recipes_by_name` | `query` | Performs a case-insensitive literal substring search. |
| `search_recipes_by_ingredient_text` | `query` | Finds recipes with an ingredient containing the query text. |
| `search_recipes_by_ingredients` | `ingredients` | Returns recipes containing every supplied ingredient. |

Names and ingredients are trimmed when recipes are created. `search_recipes_by_ingredient_text` uses case-insensitive literal substring matching, so `"egg"` matches both `"1 egg"` and `"2 eggs"`. The multi-ingredient search uses case-insensitive exact strings: `"2 eggs"` matches `"2 Eggs"`, but not `"eggs"`. Recipe IDs are stable numeric identifiers. Empty searches return an empty `recipes` array.

## Hermes Agent configuration

Hermes launches local MCP servers as subprocesses using stdio. Add an entry like this to `~/.hermes/config.yaml`:

```yaml
mcp_servers:
  recetor:
    command: "/absolute/path/to/recetor"
    args: ["--db", "/absolute/path/to/recipes.db"]
```

Then start or reload Hermes so it discovers the seven tools. This configuration is only an integration example; `recetor` itself contains no Hermes-specific code.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
```
