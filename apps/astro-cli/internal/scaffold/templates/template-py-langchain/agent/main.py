"""
{{.Name}} - {{.Description}}

Environment variables (automatically injected by 'astro dev'):
{{- range .AgentEnvVars}}
  {{.Key}} - {{.Description}}
{{- end}}
"""

{{- if .HasIntegration "anthropic"}}
from langchain_anthropic import ChatAnthropic
{{- else if .HasIntegration "openai"}}
from langchain_openai import ChatOpenAI
{{- else}}
from langchain_anthropic import ChatAnthropic
{{- end}}
from langchain.agents import create_agent
from astropods_adapter_langchain import LangChainAdapter, serve

{{- if .HasIntegration "anthropic"}}
llm = ChatAnthropic(model="claude-sonnet-4-5")
{{- else if .HasIntegration "openai"}}
llm = ChatOpenAI(model="gpt-4o")
{{- else}}
llm = ChatAnthropic(model="claude-sonnet-4-5")
{{- end}}

system_prompt = "You are {{.Name | humanName}}, a helpful AI assistant. {{.Description | pyStr}}"
agent = create_agent(llm, tools=[], system_prompt=system_prompt)

adapter = LangChainAdapter(agent, name="{{.Name}}", system_prompt=system_prompt)
serve(adapter)
