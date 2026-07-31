package add

// ValidModelProviders lists accepted providers for "ast add model".
var ValidModelProviders = []string{"anthropic", "openai", "google", "cohere"}

// ValidKnowledgeProviders lists accepted providers for "ast add knowledge".
var ValidKnowledgeProviders = []string{"qdrant", "redis", "postgres", "neo4j", "pinecone"}

// ValidIntegrationProviders lists accepted providers for "ast add tool".
var ValidIntegrationProviders = []string{"github", "gitlab"}

type option struct {
	label string
	value string
}

func triggerOptions() []option {
	return []option{
		{"Schedule (cron)", "schedule"},
		{"On startup", "startup"},
		{"Manual trigger", "manual"},
		{"Webhook", "webhook"},
	}
}

func scopeOptions() []option {
	return []option{
		{"Models", "models"},
		{"Knowledge stores", "knowledge"},
		{"Integrations", "integrations"},
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
