"""
{{.Name}} - {{.Description}}

Environment variables (automatically injected by 'astro dev'):
{{- range .AgentEnvVars}}
  {{.Key}} - {{.Description}}
{{- end}}
"""

{{- if .AIGateway}}
import os
from langchain_openai import ChatOpenAI
{{- else if .HasIntegration "anthropic"}}
from langchain_anthropic import ChatAnthropic
{{- else if .HasIntegration "openai"}}
from langchain_openai import ChatOpenAI
{{- else}}
from langchain_anthropic import ChatAnthropic
{{- end}}
from langchain.agents import create_agent
from astropods_adapter_langchain import LangChainAdapter, serve

{{- if .AIGateway}}
# Astro AI Gateway: managed model access over the OpenAI-compatible API.
# No provider key needed — the platform injects URL + credential at runtime.
llm = ChatOpenAI(
    api_key=os.environ["ASTRO_GATEWAY_API_KEY"],
    base_url=f"{os.environ['ASTRO_GATEWAY_URL']}/v1",
    # Model chosen at deploy time from the gateway options; injected as MODEL_DEFAULT.
    model=os.environ.get("MODEL_DEFAULT", "claude-sonnet-4-6"),
)
{{- else if .HasIntegration "anthropic"}}
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
