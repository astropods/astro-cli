## Summary

The `--lang` flag on `ast create` required users to know which language corresponded to which template, creating unnecessary indirection. Since each template uniquely implies a language (`mastra` → TypeScript, `langchain` → Python), the flag was redundant.

## Design

The `--lang` / `-l` flag is removed from `ast create`. The template flag (`--template` / `-t`) now fully determines the project language. A `templateToLang` map in `cmd/create.go` maps each template name to its language, which is passed to `GenerateFiles` internally. The default template remains `mastra`.

If an unrecognized template is provided, the error message includes the full list of available templates:

```
unknown template: "foo"

Available templates:
  mastra     TypeScript/Bun agent using Mastra
  langchain  Python agent using LangChain
```

## Migration

Replace any use of `--lang` with the equivalent `--template`:

- `ast create my-agent --lang py` → `ast create my-agent --template langchain`
- `ast create my-agent --lang ts` → `ast create my-agent` (mastra is the default)
