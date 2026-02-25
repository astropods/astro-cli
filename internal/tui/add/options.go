package add

// ValidModelProviders lists accepted providers for "ast add model".
var ValidModelProviders = []string{"anthropic", "openai", "google", "cohere", "ollama"}

// ValidKnowledgeProviders lists accepted providers for "ast add knowledge".
var ValidKnowledgeProviders = []string{"qdrant", "redis", "postgres", "neo4j", "pinecone"}

// ValidToolProviders lists accepted providers for "ast add tool".
var ValidToolProviders = []string{"github", "gitlab"}

type option struct {
	label string
	value string
}

func ollamaModelOptions() []option {
	return []option{
		{"llama3.2:1b", "llama3.2:1b"},
		{"llama3.1:8b", "llama3.1:8b"},
		{"mistral:7b", "mistral:7b"},
		{"codellama:7b", "codellama:7b"},
		{"phi3:3.8b", "phi3:3.8b"},
		{"gemma2:2b", "gemma2:2b"},
	}
}

func triggerOptions() []option {
	return []option{
		{"Schedule (cron)", "schedule"},
		{"On startup", "startup"},
		{"Manual trigger", "manual"},
		{"Webhook", "webhook"},
	}
}

func persistentOptions() []option {
	return []option{
		{"No", "false"},
		{"Yes", "true"},
	}
}

func scopeOptions() []option {
	return []option{
		{"Models", "models"},
		{"Knowledge stores", "knowledge"},
		{"Tools", "tools"},
	}
}

func secretOptions() []option {
	return []option{
		{"No (plain text)", "false"},
		{"Yes (stored securely)", "true"},
	}
}

func addAnotherOptions() []option {
	return []option{
		{"No, I'm done", "false"},
		{"Yes, add another", "true"},
	}
}
