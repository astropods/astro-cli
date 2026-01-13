# Project Overview

astro is a platform for building AI agents and AI powered workflows. It is a monorepo powered by Bun that has several packages and backend and frontend clients.

# Packages

## @astro/types

Contains the shared types for the project. It's recommended to define types in this package and export them to avoid circular dependencies between packages.

## @astro/graph

A class for defining graph structures with a simple DSL. A graph is a series of nodes and edges, which can become an executable program. The point of the DSL is to make it easy for developers and AI agents to quickly define executable logic that can also be represented visually as a graph. It's recommended when building these graphs to chunk them into easily digestible parts when laid out visually.

## @astro/nodes

Contains definitions for nodes that are globally available to be used in graphs. These should be considered low-level primitives. For higher-level graphs, you can create a graph that can be consumed as a module inside of other graphs. Thank you.

## @astro/workflows

In common parlance, an AI agent contains a prompt, a set of tools the agent may call, and access to any resources or external data for context. The workflows package contains reusable tools represented as graphs that will be offered as part of building an agent to both internal developers and the end user.

## @astro/agents

Contains utilities to define agents. Has collections of tools, prompts, and resources. Also contains logic for running agents.

## @astro/engine

Exports a runtime to execute graphs. It takes in input data, executes the graph, and returns a result.

# Apps

## @astro/client

A Vite React frontend application that allows you to define and run agents.
