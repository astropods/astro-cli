"""
Custom tool for searching internal documentation.

This is referenced in astroai.yml as:
  tools:
    doc_search:
      type: function
      config:
        runtime: python
        handler: tools/search.py
"""

from typing import Dict, Any, List


def search_docs(query: str, max_results: int = 5) -> List[Dict[str, Any]]:
    """
    Search internal documentation.

    Args:
        query: Search query string
        max_results: Maximum number of results to return

    Returns:
        List of document results with title, content, and relevance score
    """
    # In a real implementation, this would:
    # 1. Embed the query using the sentence-transformers model
    # 2. Query Qdrant for similar vectors
    # 3. Return formatted results

    # For now, return mock results to demonstrate the interface
    return [
        {
            "title": "Anthropic SDK - Getting Started",
            "content": "The Anthropic SDK provides a simple interface for Claude API...",
            "url": "https://docs.anthropic.com/getting-started",
            "score": 0.95
        },
        {
            "title": "Message API Reference",
            "content": "The Messages API endpoint accepts messages and returns Claude's response...",
            "url": "https://docs.anthropic.com/api/messages",
            "score": 0.88
        },
        {
            "title": "Streaming Responses",
            "content": "Claude supports streaming responses for real-time output...",
            "url": "https://docs.anthropic.com/streaming",
            "score": 0.82
        }
    ][:max_results]


# Tool metadata for the agent to understand how to call this function
TOOL_SPEC = {
    "name": "search_docs",
    "description": "Search internal engineering documentation to find relevant information",
    "parameters": {
        "type": "object",
        "properties": {
            "query": {
                "type": "string",
                "description": "The search query"
            },
            "max_results": {
                "type": "integer",
                "description": "Maximum number of results (default: 5)",
                "default": 5
            }
        },
        "required": ["query"]
    }
}
