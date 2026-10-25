# Step 5 — Configuration

## Objective

Load and save a small, versioned JSON configuration without adding a
configuration framework. Never lose unrelated user data.

## Package

`internal/config`

## Schema (version 1)

```json
{
  "schemaVersion": 1,
  "managedRoot": "~/.local/share/hilt/dotnet",
  "defaultProjectDir": "",
  "recentProjects": [],
  "scanExclusions": ["bin", "obj", ".git", "node_modules"],
  "editor": "",
  "ui": {
    "theme": "auto",
    "vimKeys": true
  }
}
```

## API

```go
type Config struct { ... }
type Store struct{ Path string }

func Defaults(managedRoot string) Config
func (Store) Load() (Config, []string, error)
func (Store) Save(Config) error
```

## Behavior

- Missing file returns defaults and no warnings. It is not created until a
  save is requested.
- Malformed JSON:
  - returns defaults;
  - renames the bad file to `config.json.corrupt-<unix-timestamp>`;
  - returns a warning describing both actions.
- An unsupported newer schema:
  - returns defaults;
  - leaves the file untouched and reports its path in the warning, so a newer
    Hilt version cannot lose settings.
- Unknown JSON fields are preserved on save where practical; Hilt must not
  silently discard settings it does not understand.
- Saves are atomic: write a mode-`0600` temp file in the same directory, sync,
  then rename.
- `SchemaVersion == 0` is upgraded to 1 with a warning.
- `ManagedRoot` defaults when empty.

## Tests

- Missing file → defaults.
- Round-trip preserves values and unknown fields.
- Corrupt file → backup exists, defaults returned, warning present.
- Unsupported future schema → warning, no data loss.
- Saved file has mode `0600`.

## Acceptance

`hilt doctor` reports config health. A deliberately corrupted config does not
prevent `hilt` from starting.
